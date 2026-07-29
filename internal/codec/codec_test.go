// Tests for the phrase codec: round-trip fidelity, compression correctness,
// and message/response helper functions.
//
// Author: Justin Campbell
package codec

import (
	"strings"
	"testing"

	"conductor/internal/api"
)

var c = New()

// ── round-trip ────────────────────────────────────────────────────────────────

func TestRoundTrip_VerboseAgentResponse(t *testing.T) {
	original := "Based on the information provided, I would like to analyze the following aspects. " +
		"Furthermore, it is important to note that the implementation should handle edge cases. " +
		"In conclusion, the approach I will adopt is straightforward."

	compressed := c.Compress(original)
	restored := c.Decompress(compressed)

	if restored != original {
		t.Errorf("round-trip failed.\noriginal:   %q\ncompressed: %q\nrestored:   %q",
			original, compressed, restored)
	}
}

func TestRoundTrip_PlainText(t *testing.T) {
	original := "This sentence contains no known phrases and should be unchanged."
	if c.Decompress(c.Compress(original)) != original {
		t.Error("round-trip changed plain text")
	}
}

func TestRoundTrip_EmptyString(t *testing.T) {
	if c.Decompress(c.Compress("")) != "" {
		t.Error("empty string round-trip failed")
	}
}

// ── compress ──────────────────────────────────────────────────────────────────

func TestCompress_KnownPhrase(t *testing.T) {
	got := c.Compress("Based on the information provided, the answer is 42.")
	if !strings.Contains(got, "<<BAIP>>") {
		t.Errorf("expected <<BAIP>> token, got %q", got)
	}
	if strings.Contains(got, "Based on the information provided,") {
		t.Errorf("original phrase should be replaced, got %q", got)
	}
}

func TestCompress_MultiplePhrasesInOneSentence(t *testing.T) {
	// "Furthermore," starts sentence → capital F; "in order to" is mid-sentence → lowercase i.
	// "please note that" is mid-sentence → lowercase p.
	got := c.Compress("Furthermore, in order to proceed, please note that you must validate first.")
	if !strings.Contains(got, "<<FUR>>") {
		t.Errorf("expected <<FUR>>, got %q", got)
	}
	if !strings.Contains(got, "<<iot>>") {
		t.Errorf("expected <<iot>> (mid-sentence), got %q", got)
	}
	if !strings.Contains(got, "<<pnt>>") {
		t.Errorf("expected <<pnt>> (mid-sentence), got %q", got)
	}
}

func TestRoundTrip_MidSentenceLowercase(t *testing.T) {
	original := "The system works; however, in order to succeed, you must also validate the input."
	if got := c.Decompress(c.Compress(original)); got != original {
		t.Errorf("lowercase round-trip failed.\noriginal: %q\ngot:      %q", original, got)
	}
}

func TestCompress_ShortensTotalLength(t *testing.T) {
	original := "Based on the information provided, I would like to analyze the following data. " +
		"Furthermore, it is important to note that the results are significant. " +
		"In conclusion, the answer is clear."
	compressed := c.Compress(original)
	if len(compressed) >= len(original) {
		t.Errorf("expected compression to shorten text: original=%d compressed=%d",
			len(original), len(compressed))
	}
	t.Logf("ratio: %.2f  (%d→%d chars)", Ratio(original, compressed), len(original), len(compressed))
}

// ── decompress ────────────────────────────────────────────────────────────────

func TestDecompress_KnownToken(t *testing.T) {
	got := c.Decompress("<<BAIP>> the analysis is complete.")
	if !strings.HasPrefix(got, "Based on the information provided,") {
		t.Errorf("expected expansion of <<BAIP>>, got %q", got)
	}
}

func TestDecompress_NoOpOnPlainText(t *testing.T) {
	text := "This is ordinary text with no shorthand tokens."
	if c.Decompress(text) != text {
		t.Error("decompressing plain text should be a no-op")
	}
}

// ── messages ──────────────────────────────────────────────────────────────────

func TestCompressMessages_SkipsSystemRole(t *testing.T) {
	msgs := []api.Message{
		{Role: "system", Content: "Based on the information provided, you are a helpful assistant."},
		{Role: "user", Content: "Based on the information provided, what should I do?"},
	}
	out := c.CompressMessages(msgs)

	if out[0].Content != msgs[0].Content {
		t.Errorf("system message should be unchanged: %q", out[0].Content)
	}
	if !strings.Contains(out[1].Content, "<<BAIP>>") {
		t.Errorf("user message should be compressed: %q", out[1].Content)
	}
}

func TestDecompressMessages_SkipsSystemRole(t *testing.T) {
	msgs := []api.Message{
		{Role: "system", Content: "<<BAIP>> you are a helpful assistant."},
		{Role: "user", Content: "<<BAIP>> what should I do?"},
	}
	out := c.DecompressMessages(msgs)

	// System message with a token in it should NOT be decompressed
	if out[0].Content != msgs[0].Content {
		t.Errorf("system message should be unchanged: %q", out[0].Content)
	}
	// User message should be decompressed
	if strings.Contains(out[1].Content, "<<BAIP>>") {
		t.Errorf("user message token should be expanded: %q", out[1].Content)
	}
}

func TestMessages_DoNotMutateOriginal(t *testing.T) {
	original := []api.Message{
		{Role: "user", Content: "Based on the information provided, what should I do?"},
	}
	_ = c.CompressMessages(original)
	if strings.Contains(original[0].Content, "<<") {
		t.Error("CompressMessages mutated original slice")
	}
}

// ── response ──────────────────────────────────────────────────────────────────

func TestCompressResponse_CompressesChoices(t *testing.T) {
	resp := &api.ChatCompletionResponse{
		Choices: []api.ChatCompletionChoice{
			{Message: api.Message{Role: "assistant", Content: "Certainly! Based on the information provided, I will now explain the approach."}},
		},
	}
	out := c.CompressResponse(resp)

	if out == resp {
		t.Error("CompressResponse should return a copy, not the same pointer")
	}
	if out.Choices[0].Message.Content == resp.Choices[0].Message.Content {
		t.Error("expected choices content to be compressed")
	}
	if resp.Choices[0].Message.Content != "Certainly! Based on the information provided, I will now explain the approach." {
		t.Error("CompressResponse mutated original response")
	}
}

// ── ratio ─────────────────────────────────────────────────────────────────────

func TestRatio_EmptyString(t *testing.T) {
	if Ratio("", "") != 1.0 {
		t.Error("ratio of empty strings should be 1.0")
	}
}

func TestRatio_Compressed(t *testing.T) {
	original := "Based on the information provided, I would like to discuss this."
	compressed := c.Compress(original)
	r := Ratio(original, compressed)
	if r >= 1.0 {
		t.Errorf("expected ratio < 1.0, got %.3f", r)
	}
}
