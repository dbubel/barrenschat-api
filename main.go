package main

import (
	"io"
	"log"
	"net/http"
	"os"

	"github.com/engineerbeard/barrenschat-api/handler"
	"github.com/engineerbeard/barrenschat-api/hub"
)

// TODO: benchcmp
func main() {
	f, err := os.OpenFile("hub_log.txt", os.O_APPEND|os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	mw := io.MultiWriter(os.Stdout, f)
	log.SetOutput(mw)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	hubHandle := hub.NewHub()
	go hubHandle.Run()

	serverMux := handler.GetEngine(hubHandle)
	log.Println("Server running")
	log.Fatalln(http.ListenAndServe(":9000", serverMux))
}
