package deviceauth

import "testing"

func TestNormalizeUserCodeAcceptsTypingVariants(t *testing.T) {
	for input, want := range map[string]string{
		"5OYE-IALT":  "5OYE-IALT",
		"5oye-ialt":  "5OYE-IALT",
		" 5OYEIALT ": "5OYE-IALT",
		"5oye ialt":  "5OYE-IALT",
		"50YE-1ALT":  "5OYE-IALT",
		"8CDE-FGHI":  "BCDE-FGHI",
		"ABCD-EFG":   "",
		"ABCD-EFGHI": "",
		"ABCD-EFG9":  "",
		"ABCD_EFGH":  "",
		"":           "",
		"ÄBCD-EFGH":  "",
	} {
		if got := NormalizeUserCode(input); got != want {
			t.Errorf("NormalizeUserCode(%q) = %q, want %q", input, got, want)
		}
	}
}
