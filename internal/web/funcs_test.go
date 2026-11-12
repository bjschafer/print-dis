package web

import "testing"

func TestInitials(t *testing.T) {
	for in, want := range map[string]string{
		"Ada Lovelace": "AL", "ada": "A", "Mary Ann O'Neil": "MN", "bob@example.com": "B",
		"jean-luc.picard@x": "JP", "": "?", "  ": "?", "élodie durand": "ÉD",
	} {
		if got := initials(in); got != want {
			t.Errorf("initials(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAvatarHue(t *testing.T) {
	if avatarHue("Ada") != avatarHue(" ada ") {
		t.Error("hue depends on case or spacing")
	}
	seen := map[string]bool{}
	for _, n := range []string{"Ada", "Bob", "Cleo", "Dev", "Eve", "Finn"} {
		seen[string(avatarHue(n))] = true
	}
	if len(seen) < 4 {
		t.Errorf("six names share only %d hues", len(seen))
	}
}
