package main

import (
	"flag"
	"fmt"
	"log"
	"search_engine/www"
)

func main() {
	content := flag.String("content", "", "content to index")
	flag.Parse()
	if *content == "" {
		log.Fatal("content is required")
	}
	www, err := www.NewWWWFromPath(*content)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(www)
}
