package main

import (
	"fmt"
	"log"
	"os"

	"github.com/kputrajaya/uplass/utils"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Provide the secret as the first argument")
	}

	hashed, err := utils.HashSecret(os.Args[1])
	if err != nil {
		log.Fatal("Can't hash secret", err)
	}

	fmt.Println(hashed)
}
