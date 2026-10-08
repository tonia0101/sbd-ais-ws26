package main

import "fmt"

type Person struct {
	Name    string
	Message string
}

func (p Person) Greet() {
	fmt.Printf("Hello World! %s says: '%s'\n", p.Name, p.Message)
}

func main() {
	user := Person{
		Name:    "Antonia",
		Message: "Hiii",
	}
	user.Greet()
}
