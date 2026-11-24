// A second, smaller way to use what this repo already does.
// Kept separate from infrai_queue.go so the main path stays as short as it was.
// Reads INFRAI_API_KEY from the environment, same as the main example.
package main

import (
	"fmt"
	"os"
)

// secondaryExample mirrors the main flow with the smallest possible surface.
func secondaryExample() {
	if os.Getenv("INFRAI_API_KEY") == "" {
		fmt.Println("set INFRAI_API_KEY first")
		return
	}
	fmt.Println("key loaded; reuse the helper from the main example here")
}
