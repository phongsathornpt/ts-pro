package wintertc_test

import (
	"testing"
)

func TestLinuxAMD64WinterTCURLPattern(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_url_pattern_basic",
		source: `
const p = new URLPattern("https://example.com/books/:id");
const r = p.exec("https://example.com/books/42");
console.log(r.pathname.input);
const g: any = r.pathname.groups;
console.log(g.id);
`,
		expected: "/books/42\n42\n",
	})
}

func TestLinuxAMD64WinterTCURLPatternFullConformance(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_url_pattern_conformance",
		source:   mustReadExample(t, "../../examples/wintertc/url_pattern_conformance.ts"),
		expected: "https\nexample.com\n8080\n/books/:id\nsort=:sort\nsection\nfalse\nhttps\nexample.org\n/api/:version/*\n*\n*\n/users/:id\nhttps\nexample.net\n3000\n/articles/:slug\nTypeError\nTypeError\ntrue\nfalse\nfalse\ntrue\nfalse\ntrue\ntrue\nfalse\ntrue\nfalse\n/books/42\n42\nasc\n1\nhttps://example.com:8080/books/42?sort=asc#section\nv1\nposts/100\ntrue\ntrue\ntrue\nfalse\n777\ntrue\ntrue\ntrue\nfalse\napi\n",
	})
}
