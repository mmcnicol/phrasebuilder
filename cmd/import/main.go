// Command import checks course content, generates its audio and loads it
// into the database.
//
// Usage:
//
//	go run ./cmd/import [flags] validate   check content/<lang>/*.yaml
//	go run ./cmd/import [flags] audio      make missing MP3s in the bucket
//	go run ./cmd/import [flags] load       migrate the database and load content
//	go run ./cmd/import [flags] all        validate, audio, load
//	go run ./cmd/import [flags] samples    write voice samples to try out
//
// validate prints each problem with its file and line, and exits with
// status 1 if there are any. Every other command validates first.
//
// audio and samples use Google Cloud Application Default Credentials
// (gcloud auth application-default login). load reads DATABASE_URL from
// the environment or a .env file.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/joho/godotenv"

	"github.com/mmcnicol/phrasebuilder/internal/audio"
	"github.com/mmcnicol/phrasebuilder/internal/content"
	"github.com/mmcnicol/phrasebuilder/internal/store"
	"github.com/mmcnicol/phrasebuilder/internal/store/migrations"
)

const sampleText = "Bonjour madame. Je voudrais un café crème et un croissant, s'il vous plaît. Où sont les toilettes ?"

func main() {
	contentDir := flag.String("content", "content", "content directory")
	bucket := flag.String("bucket", "phrasebuilder-audio", "Cloud Storage bucket for audio")
	samplesOut := flag.String("out", "voice-samples", "samples: directory to write MP3s to")
	samplesMatch := flag.String("match", "", "samples: only voices whose name matches this regexp")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: import [flags] validate|audio|load|all|samples")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	godotenv.Load() // optional; real environment variables take precedence
	ctx := context.Background()

	c := mustBuild(*contentDir)
	if m := audio.MissingVoices(c); len(m) > 0 {
		fatal(fmt.Errorf("no voice in internal/audio for languages %v", m))
	}

	switch flag.Arg(0) {
	case "validate":
		fmt.Printf("ok: %d units, %d patterns, %d sentences, %d words\n",
			len(c.Units), len(c.Patterns), len(c.Sentences), len(c.Words))
	case "audio":
		fatal(genAudio(ctx, c, *bucket))
	case "load":
		fatal(load(ctx, c))
	case "all":
		fatal(genAudio(ctx, c, *bucket))
		fatal(load(ctx, c))
	case "samples":
		fatal(samples(ctx, *samplesOut, *samplesMatch))
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

func genAudio(ctx context.Context, c *content.Course, bucketName string) error {
	tts, err := audio.NewGoogleTTS(ctx)
	if err != nil {
		return err
	}
	defer tts.Close()
	b, err := audio.NewGCSBucket(ctx, bucketName)
	if err != nil {
		return err
	}
	defer b.Close()

	clips := audio.Clips(c)
	fmt.Printf("audio: %d clips\n", len(clips))
	st, err := audio.Generate(ctx, clips, tts, b, func(cl audio.Clip) {
		fmt.Printf("  %s  %s\n", cl.Path, cl.Text)
	})
	fmt.Printf("audio: %d created (%d characters), %d already there\n", st.Created, st.Characters, st.Existing)
	return err
}

func load(ctx context.Context, c *content.Course) error {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return fmt.Errorf("DATABASE_URL is not set (put it in .env)")
	}
	applied, err := migrations.Up(ctx, url)
	for _, m := range applied {
		fmt.Printf("migrated: %s\n", m)
	}
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	s, err := store.Open(ctx, url)
	if err != nil {
		return err
	}
	defer s.Close()
	st, err := s.LoadCourse(ctx, c, audio.Path)
	if err != nil {
		return fmt.Errorf("load: %w", err)
	}
	fmt.Printf("loaded: %d units, %d patterns, %d sentences, %d words; %d rows retired\n",
		st.Units, st.Patterns, st.Sentences, st.Words, st.Retired)
	return nil
}

// samples writes sampleText in each matching fr-FR voice, to choose one.
func samples(ctx context.Context, out, match string) error {
	re, err := regexp.Compile(match)
	if err != nil {
		return err
	}
	tts, err := audio.NewGoogleTTS(ctx)
	if err != nil {
		return err
	}
	defer tts.Close()
	names, err := tts.VoiceNames(ctx, "fr-FR")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	n := 0
	for _, name := range names {
		if !strings.HasPrefix(name, "fr-FR-") || !re.MatchString(name) {
			continue
		}
		mp3, err := tts.Synthesize(ctx, audio.Voice{LanguageCode: "fr-FR", Name: name}, sampleText)
		if err != nil {
			fmt.Printf("  %s: %v\n", name, err)
			continue
		}
		path := filepath.Join(out, name+".mp3")
		if err := os.WriteFile(path, mp3, 0o644); err != nil {
			return err
		}
		fmt.Println(" ", path)
		n++
	}
	fmt.Printf("%d samples (%d characters each) of: %s\n", n, len([]rune(sampleText)), sampleText)
	return nil
}

func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "import:", err)
		os.Exit(1)
	}
}
