package audio

import (
	"context"
	"errors"

	"cloud.google.com/go/storage"
	texttospeech "cloud.google.com/go/texttospeech/apiv1"
	"cloud.google.com/go/texttospeech/apiv1/texttospeechpb"
)

// GoogleTTS synthesizes speech with Google Cloud Text-to-Speech, using
// Application Default Credentials.
type GoogleTTS struct {
	c *texttospeech.Client
}

// NewGoogleTTS connects to Text-to-Speech.
func NewGoogleTTS(ctx context.Context) (*GoogleTTS, error) {
	c, err := texttospeech.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	return &GoogleTTS{c: c}, nil
}

// Close closes the connection.
func (g *GoogleTTS) Close() error { return g.c.Close() }

// Synthesize returns MP3 audio of text spoken by v.
func (g *GoogleTTS) Synthesize(ctx context.Context, v Voice, text string) ([]byte, error) {
	resp, err := g.c.SynthesizeSpeech(ctx, &texttospeechpb.SynthesizeSpeechRequest{
		Input:       &texttospeechpb.SynthesisInput{InputSource: &texttospeechpb.SynthesisInput_Text{Text: text}},
		Voice:       &texttospeechpb.VoiceSelectionParams{LanguageCode: v.LanguageCode, Name: v.Name},
		AudioConfig: &texttospeechpb.AudioConfig{AudioEncoding: texttospeechpb.AudioEncoding_MP3},
	})
	if err != nil {
		return nil, err
	}
	return resp.AudioContent, nil
}

// VoiceNames lists the voices available for a language code such as fr-FR.
func (g *GoogleTTS) VoiceNames(ctx context.Context, languageCode string) ([]string, error) {
	resp, err := g.c.ListVoices(ctx, &texttospeechpb.ListVoicesRequest{LanguageCode: languageCode})
	if err != nil {
		return nil, err
	}
	var names []string
	for _, v := range resp.Voices {
		names = append(names, v.Name)
	}
	return names, nil
}

// GCSBucket stores audio in a Cloud Storage bucket. Files never change, so
// browsers and CDNs may cache them for good.
type GCSBucket struct {
	c *storage.Client
	b *storage.BucketHandle
}

// NewGCSBucket opens the named bucket.
func NewGCSBucket(ctx context.Context, name string) (*GCSBucket, error) {
	c, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	return &GCSBucket{c: c, b: c.Bucket(name)}, nil
}

// Close closes the connection.
func (g *GCSBucket) Close() error { return g.c.Close() }

// Exists reports whether path is in the bucket.
func (g *GCSBucket) Exists(ctx context.Context, path string) (bool, error) {
	_, err := g.b.Object(path).Attrs(ctx)
	if errors.Is(err, storage.ErrObjectNotExist) {
		return false, nil
	}
	return err == nil, err
}

// Put writes mp3 to path.
func (g *GCSBucket) Put(ctx context.Context, path string, mp3 []byte) error {
	w := g.b.Object(path).NewWriter(ctx)
	w.ContentType = "audio/mpeg"
	w.CacheControl = "public, max-age=31536000, immutable"
	if _, err := w.Write(mp3); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}
