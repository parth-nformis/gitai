package main

import (
	"fmt"
	"os"
)

// uninstall deletes the gitai binary. Requires root.
func uninstall() {
	execPath, err := os.Executable()
	if err != nil {
		fmt.Printf("Could not determine binary path: %v\n", err)
		os.Exit(1)
	}

	if os.Geteuid() != 0 {
		fmt.Printf("Cannot uninstall: not running as root.\nRun: sudo %s -uninstall\n", execPath)
		os.Exit(1)
	}

	fmt.Println("Uninstalling GitAI...")
	if err := os.Remove(execPath); err != nil {
		fmt.Printf("Uninstall failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("GitAI uninstalled successfully!")
}
