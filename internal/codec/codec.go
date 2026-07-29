// Package codec provides a symmetric shorthand codec for agent-to-agent traffic.
// Verbose LLM preamble and transition phrases are replaced with compact tokens
// so conversation history shrinks across multi-turn agent interactions.
// The codec is purely rule-based; Phase 7 can replace it with a fine-tuned model.
//
// Author: Justin Campbell
package codec

import (
	"strings"

	"conductor/internal/api"
)

// Token format: <<ABBR>> for sentence-starting (capitalized) phrases,
// <<abbr>> for mid-sentence (lowercase) variants.
// Every token is unique so Decompress is an exact inverse of Compress.
//
// Pairs are ordered longest-first within shared prefixes so that the more
// specific phrase matches before a shorter one that starts the same way.
var pairs = []struct{ natural, short string }{
	// ── Openers (sentence-start only) ──────────────────────────────────────
	{"I'd be happy to help with that! ", "<<HLP2>>"},
	{"I'd be happy to help! ", "<<HLP>>"},
	{"In response to your question, ", "<<IRYQ>>"},
	{"Based on the information provided, ", "<<BAIP>>"},
	{"Based on the context provided, ", "<<BACP>>"},
	{"Based on your request, ", "<<BAYR>>"},
	{"Based on the context, ", "<<BACT>>"},
	{"Based on the above, ", "<<BAAB>>"},
	{"Certainly! ", "<<CRT>>"},
	{"Of course! ", "<<OCR>>"},
	{"Sure! ", "<<SUR>>"},

	// ── Hedges / caveats ────────────────────────────────────────────────────
	{"It is important to note that ", "<<IIN>>"},
	{"It's important to note that ", "<<IIN2>>"},
	{"It's worth noting that ", "<<IWN>>"},
	{"Please keep in mind that ", "<<PKM>>"},
	{"Please note that ", "<<PNT>>"},
	{"please note that ", "<<pnt>>"},

	// ── Discourse connectors – capitalized (sentence-start) ─────────────────
	{"With that in mind, ", "<<WTIM>>"},
	{"That being said, ", "<<TBS>>"},
	{"Having said that, ", "<<HST>>"},
	{"As mentioned earlier, ", "<<AME>>"},
	{"As previously stated, ", "<<APS>>"},
	{"On the other hand, ", "<<OOH>>"},
	{"In conclusion, ", "<<INC>>"},
	{"In summary, ", "<<ISM>>"},
	{"To summarize, ", "<<SUM>>"},
	{"In other words, ", "<<IOW>>"},
	{"As a result, ", "<<AAR>>"},
	{"Nevertheless, ", "<<NVL>>"},
	{"Furthermore, ", "<<FUR>>"},
	{"Additionally, ", "<<ADD>>"},
	{"Moreover, ", "<<MOR>>"},
	{"However, ", "<<HOW>>"},
	{"For instance, ", "<<FIN>>"},
	{"For example, ", "<<FEX>>"},
	{"In order to ", "<<IOT>>"},
	{"In this case, ", "<<ITC>>"},
	{"At this point, ", "<<ATP>>"},

	// ── Discourse connectors – lowercase (mid-sentence) ─────────────────────
	{"in other words, ", "<<iow>>"},
	{"as a result, ", "<<aar>>"},
	{"nevertheless, ", "<<nvl>>"},
	{"furthermore, ", "<<fur>>"},
	{"additionally, ", "<<add>>"},
	{"moreover, ", "<<mor>>"},
	{"however, ", "<<how>>"},
	{"for instance, ", "<<fin>>"},
	{"for example, ", "<<fex>>"},
	{"in order to ", "<<iot>>"},
	{"in this case, ", "<<itc>>"},
	{"at this point, ", "<<atp>>"},

	// ── Verbose agent phrases ────────────────────────────────────────────────
	{"I would like to ", "<<IWL>>"},
	{"I am going to ", "<<IAG>>"},
	{"I have successfully ", "<<IHS>>"},
	{"I will now ", "<<IWNO>>"},
	{"I have ", "<<IH>>"},
	{"I will ", "<<IW>>"},
	{"the following ", "<<TFL>>"},
	{"as follows:", "<<AFL>>"},
	{"the above ", "<<TAB>>"},
}

// Codec applies symmetric natural ↔ shorthand substitutions to message content.
type Codec struct {
	compressor   *strings.Replacer
	decompressor *strings.Replacer
}

// New builds a Codec from the built-in phrase dictionary.
func New() *Codec {
	comp := make([]string, 0, len(pairs)*2)
	decomp := make([]string, 0, len(pairs)*2)

	seen := map[string]bool{}
	for _, p := range pairs {
		comp = append(comp, p.natural, p.short)
		if !seen[p.short] {
			decomp = append(decomp, p.short, p.natural)
			seen[p.short] = true
		}
	}

	return &Codec{
		compressor:   strings.NewReplacer(comp...),
		decompressor: strings.NewReplacer(decomp...),
	}
}

// Compress replaces verbose natural-language phrases with compact tokens.
func (c *Codec) Compress(text string) string { return c.compressor.Replace(text) }

// Decompress expands compact tokens back to natural language.
func (c *Codec) Decompress(text string) string { return c.decompressor.Replace(text) }

// CompressMessages returns a copy of msgs with non-system content compressed.
// System messages are operator prompts and must not be altered.
func (c *Codec) CompressMessages(msgs []api.Message) []api.Message {
	out := make([]api.Message, len(msgs))
	for i, m := range msgs {
		out[i] = m
		if m.Role != "system" {
			out[i].Content = c.Compress(m.Content)
		}
	}
	return out
}

// DecompressMessages returns a copy of msgs with non-system content decompressed.
func (c *Codec) DecompressMessages(msgs []api.Message) []api.Message {
	out := make([]api.Message, len(msgs))
	for i, m := range msgs {
		out[i] = m
		if m.Role != "system" {
			out[i].Content = c.Decompress(m.Content)
		}
	}
	return out
}

// CompressResponse returns a shallow copy of resp with assistant message content compressed.
func (c *Codec) CompressResponse(resp *api.ChatCompletionResponse) *api.ChatCompletionResponse {
	out := *resp
	out.Choices = make([]api.ChatCompletionChoice, len(resp.Choices))
	for i, ch := range resp.Choices {
		out.Choices[i] = ch
		out.Choices[i].Message.Content = c.Compress(ch.Message.Content)
	}
	return &out
}

// Ratio returns compressed length / original length. Values below 1.0 indicate savings.
func Ratio(original, compressed string) float64 {
	if len(original) == 0 {
		return 1.0
	}
	return float64(len(compressed)) / float64(len(original))
}
