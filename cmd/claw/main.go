package main

import (
	"fmt"
	"os"

	"github.com/oscarcode/elementary-claw/internal/app"
)

func main() {
	application, err := app.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := application.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
