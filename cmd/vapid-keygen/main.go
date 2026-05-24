package main

import (
	"fmt"
	"os"

	"github.com/dave/choresy/internal/push"
)

func main() {
	priv, pub, err := push.GenerateVAPIDKeys()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error generating VAPID keys: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("# Add these to GitHub Secrets (Settings → Secrets and variables → Actions):")
	fmt.Printf("VAPID_PRIVATE_KEY=%s\n", priv)
	fmt.Printf("VAPID_PUBLIC_KEY=%s\n", pub)
	fmt.Println("# Set this to your contact email:")
	fmt.Println("VAPID_SUBJECT=mailto:your-email@example.com")
}
