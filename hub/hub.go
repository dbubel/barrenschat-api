package hub

import (
	"encoding/json"
	"log"
	"math/rand"
	"time"

	"github.com/go-redis/redis"
	"github.com/gorilla/websocket"
)

type Message struct {
	MsgType string
	Data    map[string]interface{}
}

type Hub struct {
	NewConnection    chan *websocket.Conn
	ClientDisconnect chan *websocket.Conn
	RoomList         map[string][]*Client
	RedisClient      *redis.Client
}

type Client struct {
	conn        *websocket.Conn
	closeChan   chan int
	newMsgChan  chan string
	channelName string
}

func NewHub() *Hub {

	// TODO: fail if redis isnt started
	rand.Seed(time.Now().Unix())
	x := &Hub{
		NewConnection:    make(chan *websocket.Conn),
		ClientDisconnect: make(chan *websocket.Conn),
		RoomList:         make(map[string][]*Client),
		RedisClient: redis.NewClient(&redis.Options{
			Addr:     "redis:6379",
			Password: "", // no password set
			DB:       0,  // use default DB
		}),
	}
	log.Println("Redis:", x.RedisClient.Ping().String())
	x.listenRedis()
	return x
}

func (h *Hub) GetChannels() {
	channelList := h.RedisClient.PubSubChannels("*")
	for _, j := range channelList.Val() {
		_ = j
		//log.Debug(j)
	}
}

type msgRouter struct {
	routes map[string]func()
}

func (r *msgRouter) addRoute(route string, f func()) {
	r.routes[route] = f
}

func (h *Hub) listenRedis() {
	go func() {
		defer h.RedisClient.Close()
		pSub := h.RedisClient.Subscribe("datapipe")

		// if subscr, err := pSub.ReceiveTimeout(time.Second); err == nil {
		// 	log.Println(subscr)
		// } else {
		// 	log.Println(err.Error())
		// 	panic(err)
		// }

		for {
			msg, err := pSub.ReceiveMessage()
			if err != nil {
				log.Println(err.Error())
			}
			log.Println("New msg from datapipe:", msg.Payload)
			mmsg := Message{}
			err = json.Unmarshal([]byte(msg.Payload), &mmsg)
			if err != nil {
				log.Println(err.Error())
			}
			h.Broadcast(mmsg) // TODO: make into channel
		}
	}()
}

func (h *Hub) Broadcast(msg Message) {
	for _, j := range h.RoomList {
		for i := 0; i < len(j); i++ {
			// TODO: switch for msg type here
			if msg.MsgType == "message_new" {
				log.Println(msg.Data)
				j[i].newMsgChan <- msg.Data["msgText"].(string)
			} else {
				log.Println("Bad message type", msg)
			}
		}
	}
}

func (h *Hub) newClientConnection(c *websocket.Conn) {
	log.Printf("New client connection [%s]\n", c.RemoteAddr().String())

	closeConnChan := make(chan int)
	newClient := &Client{
		conn:        c,
		channelName: "main",
		newMsgChan:  make(chan string),
		closeChan:   closeConnChan,
	}
	h.RoomList["main"] = append(h.RoomList["main"], newClient)

	//Reader
	go func(c *websocket.Conn, h *Hub, client *Client) {

		defer func() {
			log.Printf("Closing reader for [%s]\n", c.RemoteAddr().String())
			c.Close()
			client.closeChan <- 1
			h.ClientDisconnect <- c
		}()

		for {
			_, msg, err := c.ReadMessage()

			if err != nil {
				log.Println(err.Error())
				break
			}
			err = h.RedisClient.Publish("datapipe", msg).Err()
			//log.Println("RECV:", msgType, string(msg))
		}
	}(c, h, newClient)

	// Writer
	go func(c *websocket.Conn, h *Hub, client *Client) {
		defer log.Printf("Closing writer for [%s]\n", c.RemoteAddr().String())

		ticker := time.NewTicker(time.Second * 5)
		for {
			select {
			case <-client.closeChan:
				log.Println("Stopping ticker")
				ticker.Stop()
				return
			case sendMsg := <-client.newMsgChan:
				packet := struct {
					Txt string
				}{
					sendMsg,
				}
				c.WriteJSON(packet)
			case <-ticker.C:
				err := c.WriteMessage(websocket.PingMessage, nil)
				if err != nil {
					log.Println(err.Error())
					return
				}
			}
		}
	}(c, h, newClient)
}

func (h *Hub) removeCLient(c *websocket.Conn) {
	for _, j := range h.RoomList {
		for i := 0; i < len(j); i++ {
			if c == j[i].conn {
				close(j[i].closeChan)

				close(j[i].newMsgChan)
				log.Printf("Removed [%s]\n", c.RemoteAddr().String())
				h.RoomList[j[i].channelName] = append(h.RoomList[j[i].channelName][:i], h.RoomList[j[i].channelName][i+1:]...)
				return
			}
		}
	}
}

func (h *Hub) Run() {
	for {
		select {
		case c := <-h.NewConnection:
			h.newClientConnection(c)
		case c := <-h.ClientDisconnect:
			h.removeCLient(c)
			//log.Println("New client:", c.RemoteAddr())
			// case msg := <-h.NewMessage:
			// 	log.Println("New message recv:", msg)
			// 	h.Broadcast(msg)
			// case c := <-h.ClientDisconnect:
			// 	h.removeCLient(c)
		}
	}
}
