// Command import checks course content and loads it.
//
// Usage:
//
//	go run ./cmd/import [-content dir] validate
//
// validate checks every content/<lang>/*.yaml file and prints each problem
// with its file and line. It exits with status 1 if there are any.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/mmcnicol/phrasebuilder/internal/content"
)

func main() {
	contentDir := flag.String("content", "content", "content directory")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: import [-content dir] validate")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	switch flag.Arg(0) {
	case "validate":
		c := mustBuild(*contentDir)
		fmt.Printf("ok: %d units, %d patterns, %d sentences, %d words\n",
			len(c.Units), len(c.Patterns), len(c.Sentences), len(c.Words))
	default:
		flag.Usage()
		os.Exit(2)
	}
}

// mustBuild loads and checks the content, or prints its problems and exits.
func mustBuild(dir string) *content.Course {
	units, problems := content.Load(os.DirFS(dir))
	c, more := content.Build(units)
	problems = append(problems, more...)
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintf(os.Stderr, "%s/%s\n", dir, p)
		}
		fmt.Fprintf(os.Stderr, "%d problems\n", len(problems))
		os.Exit(1)
	}
	return c
}
