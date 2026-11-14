package matching

import "testing"

func testIndex() []productIndex {
	return BuildIndex([]CatalogProduct{
		{ID: 1, Name: "Google Chrome"},
		{ID: 2, Name: "Notepad++"},
		{ID: 3, Name: "Microsoft Office"},
		{ID: 4, Name: "Adobe Photoshop"},
		{ID: 5, Name: "Slack"},
	}, []CatalogAlias{
		{ProductID: 4, Alias: "Фотошоп", Confidence: 1},
	})
}

func TestNormalizeDirtyNames(t *testing.T) {
	cases := map[string]string{
		"chrome.exe":                 "chrome",
		"Google_Chrome_v2":           "google chrome",
		"chrome_setup":               "chrome",
		"Хром экзе":                  "chrome",
		"Google Chrome (x64)":        "google chrome",
		"Notepad++":                  "notepad",
		"Microsoft Office 2013":      "microsoft office",
		"Фотошоп 2024":               "photoshop",
		"Visual C++ Redistributable": "visual redistributable",
	}
	for input, expected := range cases {
		if actual := Normalize(input); actual != expected {
			t.Fatalf("Normalize(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestBestMatchesChromeVariants(t *testing.T) {
	index := testIndex()
	for _, raw := range []string{"chrome.exe", "Google_Chrome_v2", "chrome_setup", "Хром экзе", "Google Chrome (x64)", "chromium"} {
		matches := BestMatches(raw, index, 1)
		if len(matches) == 0 {
			t.Fatalf("%q did not match any product", raw)
		}
		if matches[0].ProductID != 1 {
			t.Fatalf("%q matched product %d (%s), want Google Chrome", raw, matches[0].ProductID, matches[0].ProductName)
		}
		if matches[0].Score < matchThreshold {
			t.Fatalf("%q matched with score %.2f below threshold", raw, matches[0].Score)
		}
	}
}

func TestBestMatchesAliasAndExact(t *testing.T) {
	index := testIndex()
	photoshop := BestMatches("Фотошоп 2024", index, 1)
	if len(photoshop) != 1 || photoshop[0].ProductID != 4 {
		t.Fatalf("alias match failed: %+v", photoshop)
	}
	office := BestMatches("Microsoft Office 2013", index, 1)
	if len(office) != 1 || office[0].ProductID != 3 || office[0].Method != "exact" {
		t.Fatalf("exact match failed: %+v", office)
	}
}

func TestBestMatchesUnknown(t *testing.T) {
	if matches := BestMatches("unknown-tool-zzz", testIndex(), 3); len(matches) != 0 {
		t.Fatalf("unknown name matched: %+v", matches)
	}
}

func TestTokensRemoveNoise(t *testing.T) {
	tokens := Tokens(Normalize("Microsoft Office MUI x64 2021"))
	joined := ""
	for _, token := range tokens {
		joined += token + " "
	}
	if joined != "microsoft office mui " {
		t.Fatalf("unexpected tokens: %q", joined)
	}
}
