package main

import (
	"fmt"
	"os"
)

// uninstall deletes the gitai binary it is running from; no privileges
// needed unless it lives in a root-owned directory.
func uninstall() {
	execPath, err := os.Executable()
	if err != nil {
		fmt.Printf("Could not determine binary path: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Uninstalling GitAI...")
	if err := os.Remove(execPath); err != nil {
		fmt.Printf("Uninstall failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("GitAI uninstalled successfully!")
}
