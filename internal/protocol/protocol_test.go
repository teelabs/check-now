package protocol

import "testing"

func TestDecodeRejectsUnsupportedVersionAndMissingFields(t *testing.T) {
	if _, err := DecodeResult([]byte(`{"protocol":2,"id":"node","status":"up_to_date","tool":"Node","checked_at":"2026-01-01T00:00:00Z"}`)); err == nil {
		t.Fatal("expected unsupported protocol error")
	}
	if _, err := DecodeResult([]byte(`{"protocol":1,"id":"node","status":"up_to_date"}`)); err == nil {
		t.Fatal("expected missing fields error")
	}
}

func TestQuoteArgvUsesPOSIXSingleQuotes(t *testing.T) {
	got := QuoteArgv([]string{"bun", "install --global", "a'b", ""})
	want := "bun 'install --global' 'a'\\''b' ''"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestDecodeMetadataOmitsOptionalFieldsWithoutRejectingIt(t *testing.T) {
	m, err := DecodeMetadata([]byte(`{"protocol":1,"id":"tool","name":"Tool","description":"Checks Tool","runtime":"shell","capabilities":["refresh"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "tool" || m.Tool != "" || m.Manager != "" || m.SourceURL != "" {
		t.Fatalf("metadata=%+v", m)
	}
}

func TestDecodeRejectsTrailingJSONGarbage(t *testing.T) {
	data := `{"protocol":1,"id":"x","tool":"X","status":"unknown","checked_at":"2026-01-01T00:00:00Z"} garbage`
	if _, err := DecodeResult([]byte(data)); err == nil {
		t.Fatal("trailing garbage accepted")
	}
}

func TestDecodeRejectsMalformedCheckedAt(t *testing.T) {
	data := `{"protocol":1,"id":"x","tool":"X","status":"unknown","checked_at":"not-a-timestamp"}`
	if _, err := DecodeResult([]byte(data)); err == nil {
		t.Fatal("malformed checked_at accepted")
	}
}

func TestDecodeResultAcceptsOptionalUpdateChoices(t *testing.T) {
	r, err := DecodeResult([]byte(`{"protocol":1,"id":"node","tool":"Node","status":"update_available","current_version":"24.19.0","updates":[{"scope":"same_major","track":"24.x","version":"24.20.0","argv":["fnm","install","24.20.0"]},{"scope":"latest","track":"latest","version":"26.12.1"}],"checked_at":"2026-01-01T00:00:00Z"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Updates) != 2 || r.Updates[0].Scope != "same_major" || r.Updates[1].Version != "26.12.1" {
		t.Fatalf("updates=%+v", r.Updates)
	}
}

func TestValidateRequestAcceptsCurrentAndRefreshOnly(t *testing.T) {
	for _, mode := range []string{ModeCurrent, ModeRefresh} {
		if err := ValidateRequest(Request{Protocol: Version, ID: "tool", Mode: mode}); err != nil {
			t.Fatalf("mode %s: %v", mode, err)
		}
	}
	if err := ValidateRequest(Request{Protocol: Version, ID: "tool", Mode: "bogus"}); err == nil {
		t.Fatal("unsupported mode accepted")
	}
}
