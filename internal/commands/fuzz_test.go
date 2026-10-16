package commands

import "testing"

func FuzzParseAppURL(f *testing.F) {
	f.Add("")
	f.Add("https://mailpiece.example.com/834206594/posts/03f5v9zppxrcc2ahjcnwkp1mt")
	f.Add("https://mailpiece.example.com/834206594/posts/03f5v9zppxrcc2ahjcnwkp1mt.json?page=2")
	f.Add("https://mailpiece.example.com/my/identity")
	f.Add("https://mailpiece.example.com/834206594")
	f.Add("http://app.mailpiece.localhost:3020/834206594/subscribers")
	f.Add("ftp://mailpiece.example.com/834206594/subscribers")
	f.Add("not-a-url")
	f.Add("  https://mailpiece.example.com//834206594//posts  ")

	f.Fuzz(func(t *testing.T, raw string) {
		_, _ = parseAppURL(raw) // must not panic
	})
}

func FuzzParsePath(f *testing.F) {
	f.Add("")
	f.Add("posts.json")
	f.Add("/posts.json")
	f.Add("https://mailpiece.example.com/834206594/posts.json?page=2")
	f.Add("https://mailpiece.example.com/123/posts.json")
	f.Add("HTTPS://evil.example.com/posts.json")
	f.Add("//https://evil.example.com/posts.json")
	f.Add("https://mailpiece.example.com:8443/posts.json")
	f.Add("http://")

	f.Fuzz(func(t *testing.T, input string) {
		_, _ = parsePath(input, "https://mailpiece.example.com", "834206594") // must not panic
	})
}

func FuzzNormalizeSkillPath(f *testing.F) {
	f.Add("")
	f.Add("~/skills")
	f.Add("~/skills/mailpiece")
	f.Add("~/skills/mailpiece/SKILL.md")
	f.Add("/tmp/custom/path")
	f.Add("/tmp/custom/path/mailpiece")
	f.Add("relative/path")
	f.Add("file.md")

	f.Fuzz(func(t *testing.T, path string) {
		normalizeSkillPath(path) // must not panic
	})
}

func FuzzExpandSkillPath(f *testing.F) {
	f.Add("")
	f.Add("~")
	f.Add("~/")
	f.Add("~/foo/bar")
	f.Add(`~\foo\bar`)
	f.Add("/absolute/path")
	f.Add("relative/path")

	f.Fuzz(func(t *testing.T, path string) {
		expandSkillPath(path) // must not panic
	})
}
