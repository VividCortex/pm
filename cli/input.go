package main

import (
	"fmt"
	"os"
	"strings"
)

func inputLoop() {
	for {
		switch readKey() {
		case "k":
			paused = true

			host := ""
			id := ""
			message := ""
			fmt.Println()
			host = readString("Host: ")
			id = readString("ID: ")
			message = readString("Message: ")
			fmt.Printf("Killing ID %s on %s with message %s.\n", id, host, message)
			if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
				host = "http://" + host
			}

			client, exists := clients[host]
			if exists {
				client.Kill(id, message)
			}

			paused = false
		case "p":
			paused = !paused
		case "q":
			os.Exit(0)
		}
	}
}

func readKey() string {
	var b []byte = make([]byte, 1)
	os.Stdin.Read(b)
	return string(b)
}

func readString(prompt string) string {
	// enable input buffering (cooked mode) temporarily
	enableInputBuffering()
	defer disableInputBuffering()

	read := ""
	fmt.Print(prompt + " ")

	fmt.Scanf("%s", &read)
	return read
}
