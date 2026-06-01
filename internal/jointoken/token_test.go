package jointoken

import "testing"

func TestNormalizeRemovesWhitespace(t *testing.T) {
	got := Normalize("  goginx_join_ab c\n\tdef\r\n")
	if got != "goginx_join_abcdef" {
		t.Fatalf("unexpected normalized token %q", got)
	}
}
