package main

import (
	"fmt"
	"net/http"
)

func main() {

	serverFile := http.FileServer(http.Dir("./static"))
	http.Handle("/", serverFile)

	port := ":8080"

	fmt.Printf("Server is running on http://localhost%s\n", port)
	err := http.ListenAndServe(port, nil)
	
	if err != nil {
		fmt.Println("Error starting server:", err)
	}
}