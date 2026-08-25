package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
)

// doUpdate downloads and runs install.sh with GITAI_UPDATE=true,
// replacing the current binary while preserving ~/.gitai/ config.
func doUpdate() {
	// Check if Go is installed (required by install.sh).
	if _, err := exec.LookPath("go"); err != nil {
		fmt.Println("Error: Go is not installed. Please install Go (https://go.dev/).")
		os.Exit(1)
	}

	// Check if curl is installed.
	if _, err := exec.LookPath("curl"); err != nil {
		fmt.Println("Error: curl is not installed. Please install curl (https://curl.se/).")
		os.Exit(1)
	}

	fmt.Println("Updating GitAI...")
	scriptURL := "https://raw.githubusercontent.com/parth-nformis/gitai/main/install.sh"

	resp, err := http.Get(scriptURL)
	if err != nil {
		fmt.Printf("Failed to download install script: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Failed to download install script (status: %d)\n", resp.StatusCode)
		os.Exit(1)
	}

	script, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Failed to read install script: %v\n", err)
		os.Exit(1)
	}

	cmd := exec.Command("bash", "-")
	cmd.Env = append(os.Environ(), "GITAI_UPDATE=true")
	cmd.Stdin = bytes.NewReader(script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Println("\nUpdate failed. Run manually: curl -sL " + scriptURL + " | GITAI_UPDATE=true bash -")
		os.Exit(1)
	}
	fmt.Println("\nUpdate complete! Re-run gitai to use the new version.")
}
