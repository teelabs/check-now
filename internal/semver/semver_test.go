package semver

import "testing"

func TestPolicies(t *testing.T) {
	for _, tc := range []struct {
		name, current, target string
		want                  bool
	}{
		{"latest", "1.2.0", "1.3.0", true}, {"stable beats prerelease", "1.2.0-beta", "1.2.0", true}, {"same major", "1.2.0", "1.9.0", true}, {"different major", "1.2.0", "2.0.0", true}, {"not downgrade", "2.0.0", "1.9.0", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, e := Latest(tc.current, tc.target)
			if e != nil || got != tc.want {
				t.Fatalf("got %v %v", got, e)
			}
		})
	}
	if _, err := Parse("not-a-version"); err == nil {
		t.Fatal("malformed version accepted")
	}
}

func TestPrereleaseIdentifiersUseSemverOrdering(t *testing.T) {
	newer, err := Latest("1.0.0-alpha.2", "1.0.0-alpha.10")
	if err != nil || !newer {
		t.Fatalf("alpha.10 should follow alpha.2: newer=%v err=%v", newer, err)
	}
	newer, err = Latest("1.0.0-alpha", "1.0.0-alpha.1")
	if err != nil || !newer {
		t.Fatalf("alpha.1 should follow alpha: newer=%v err=%v", newer, err)
	}
}
