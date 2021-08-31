package main

import (
	"log"
	"sshpoll/internal/pollserver"

	"github.com/gliderlabs/ssh"
)

func main() {
	ssh.Handle(pollserver.New().Handler)
	log.Print("Server starting on port 2222. You can connect by using 'ssh -p 2222 127.0.0.1'")
	log.Fatal(ssh.ListenAndServe(":2222", nil))
}
