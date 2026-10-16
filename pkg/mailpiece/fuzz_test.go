package mailpiece

import "testing"

func FuzzParseNextLink(f *testing.F) {
	f.Add("")
	f.Add(`<https://api.example.com/page2>; rel="next"`)
	f.Add(`<https://api.example.com/page1>; rel="prev", <https://api.example.com/page3>; rel="next"`)
	f.Add(`<https://api.example.com/page1>; rel="prev"`)
	f.Add(`<>; rel="next"`)
	f.Add(`malformed header`)

	f.Fuzz(func(t *testing.T, header string) {
		parseNextLink(header) // must not panic
	})
}

func FuzzParseRetryAfter(f *testing.F) {
	f.Add("")
	f.Add("5")
	f.Add("0")
	f.Add("-1")
	f.Add("999999999")
	f.Add("not-a-number")
	f.Add("Wed, 21 Oct 2015 07:28:00 GMT")

	f.Fuzz(func(t *testing.T, value string) {
		seconds := parseRetryAfter(value)
		if seconds < 0 {
			t.Errorf("parseRetryAfter(%q) returned negative seconds: %d", value, seconds)
		}
	})
}

func FuzzDecode(f *testing.F) {
	f.Add("")
	f.Add("null")
	f.Add("{}")
	f.Add("[]")
	f.Add(`{"id":"i1","accounts":[{"id":"a1","slug":"/834206594","user":{"id":"u1","role":"owner","email_address":null}}]}`)
	f.Add(`{"accounts":{"id":"1"}}`)
	f.Add(`{"accounts":[`)

	f.Fuzz(func(t *testing.T, body string) {
		_, _, _ = decode[Identity](&Response{Data: []byte(body)}, nil) // must not panic
	})
}

func FuzzDecodeList(f *testing.F) {
	f.Add("")
	f.Add("null")
	f.Add("{}")
	f.Add("[]")
	f.Add(`[{"id":"t1","description":"Laptop","permission":"read","created_at":null}]`)
	f.Add(`[null]`)
	f.Add(`[{"id":`)

	f.Fuzz(func(t *testing.T, body string) {
		_, _, _ = decodeList[AccessToken](&Response{Data: []byte(body)}, nil) // must not panic
	})
}
