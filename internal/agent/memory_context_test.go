package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsIdentityQuery(t *testing.T) {
	cases := map[string]bool{
		"siapa nama saya":        true,
		"Siapa Nama Saya?":       true,
		"saya siapa":             true,
		"nama saya apa":          true,
		"who am i":               true,
		"what's my name":         true,
		"call me by my name":     true,
		"do you know me":         true,
		"profil saya bagaimana":  true,
		"apakah kamu kenal saya": true,
		"halo saya mau bertanya": false,
		"apa cuaca hari ini":     false,
		"saya mau coding":        false,
		"":                       false,
	}
	for input, want := range cases {
		assert.Equal(t, want, isIdentityQuery(input), "input=%q", input)
	}
}

func TestDetectIntroduction_Patterns(t *testing.T) {
	cases := []struct {
		in         string
		wantOK     bool
		wantStored string
	}{
		{"nama saya Cahya", true, "User nama: Cahya"},
		{"Nama saya Cahya Ari Pratama", true, "User nama: Cahya Ari Pratama"},
		{"nama saya Budi, dan saya kerja di X", true, "User nama: Budi"},
		{"panggil saya cahya ya", true, "Panggil user: cahya ya"},
		{"my name is Bob", true, "User name: Bob"},
		{"call me alice", true, "Call user: alice"},
		{"saya bernama Dewi.", true, "User nama: Dewi"},
		// Negatives
		{"saya mau bertanya", false, ""},
		{"halo apa kabar", false, ""},
		{"", false, ""},
		// Unrelated mention of "nama"
		{"apa nama bot ini", false, ""},
	}
	for _, c := range cases {
		got, ok := detectIntroduction(c.in)
		assert.Equal(t, c.wantOK, ok, "input=%q", c.in)
		if c.wantOK {
			assert.Equal(t, c.wantStored, got, "input=%q", c.in)
		}
	}
}

func TestDetectIntroduction_RejectsTooLong(t *testing.T) {
	long := "nama saya " + string(make([]byte, 80))
	for i := range long[10:] {
		_ = i
	}
	// 80 chars after "nama saya " — exceeds 60 char limit.
	veryLong := "nama saya AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	_, ok := detectIntroduction(veryLong)
	assert.False(t, ok, "should reject names > 60 chars")
}
