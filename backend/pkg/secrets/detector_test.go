package secrets_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/pkg/secrets"
)

func marker() string {
	sum := sha256.Sum256([]byte("craftpulse fixture marker, not a credential"))
	return base64.RawURLEncoding.EncodeToString(sum[:])[:32]
}

func safe(t *testing.T, data []byte) {
	t.Helper()
	m := marker()
	for i := 0; i+8 <= len(m); i++ {
		assert.False(t, bytes.Contains(data, []byte(m[i:i+8])), "secret fragment present")
	}
}

func TestDiscover_StandaloneAndEscapedValues(t *testing.T) {
	var encoded, entities strings.Builder
	for _, c := range marker() {
		fmt.Fprintf(&encoded, "\\u%04x", c)
		fmt.Fprintf(&entities, "&#%d;", c)
	}
	for _, tt := range []struct{ name, content string }{
		{"plain", marker()}, {"json", `{"evidence":"` + encoded.String() + `"}`}, {"xml", `<failure>` + entities.String() + `</failure>`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			detector, err := secrets.New(t.Context())
			require.NoError(t, err)
			// Untrusted report names cannot opt out through Gitleaks' filename allowlists.
			require.NoError(t, detector.Discover(t.Context(), "files/vendor/github.com/fixture/secret.png", []byte(tt.content)))
			cleaned, err := detector.Scrubber.Redact([]byte(tt.content))
			require.NoError(t, err)
			safe(t, cleaned)
			decoded := string(cleaned)
			if tt.name == "json" {
				var v map[string]string
				require.NoError(t, json.Unmarshal(cleaned, &v))
				decoded = v["evidence"]
			}
			safe(t, []byte(decoded))
			assert.Positive(t, detector.Scrubber.Replacements())
		})
	}
}

func TestScrubber_RedactedObjectKeysStayDeterministic(t *testing.T) {
	s := secrets.NewScrubber(8)
	s.Add("credential-one")
	s.Add("credential-two")
	input := []byte(`{"credential-one":"first","credential-two":"second"}`)
	first, err := s.Redact(input)
	require.NoError(t, err)
	for range 20 {
		got, err := s.Redact(input)
		require.NoError(t, err)
		assert.Equal(t, first, got)
	}
}

func TestDiscover_CleanMarkdownAutolink(t *testing.T) {
	detector, err := secrets.New(t.Context())
	require.NoError(t, err)
	content := []byte("<https://example.invalid>\n")
	require.NoError(t, detector.Discover(t.Context(), "files/README.md", content))
	safe, err := detector.Scrubber.Redact(content)
	require.NoError(t, err)
	assert.Equal(t, content, safe)
}

func TestDiscover_XMLCharacterDataIsOneLogicalValue(t *testing.T) {
	var pieces []string
	for start := 0; start < len(marker()); start += 4 {
		pieces = append(pieces, marker()[start:start+4])
	}
	input := []byte(`<failure><![CDATA[` + strings.Join(pieces, `]]><!--split--><![CDATA[`) + `]]></failure>`)
	detector, err := secrets.New(t.Context())
	require.NoError(t, err)
	require.NoError(t, detector.Discover(t.Context(), "junit.xml", input))
	cleaned, err := detector.Scrubber.Redact(input)
	require.NoError(t, err)
	var value string
	require.NoError(t, xml.Unmarshal(cleaned, &value))
	safe(t, []byte(value))
}
