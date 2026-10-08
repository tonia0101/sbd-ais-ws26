package main

import "fmt"

type Message struct {
	Text string
}

func (m Message) Print() {
	fmt.Println(m.Text)
}

func main() {
	m := Message{Text: "Hello World!"}
	m.Print()
}
