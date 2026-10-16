package commands

import "testing"

func TestPostHistoryList(t *testing.T) {
	mock, result := mockTest(t)
	mock.WithListData([]any{map[string]any{"id": "r1"}, map[string]any{"id": "r2"}})

	assertExitCode(t, postHistoryListCmd.RunE(postHistoryListCmd, []string{"p1"}), 0)

	if got := mock.GetWithPaginationCalls[0].Path; got != "/posts/p1/history.json" {
		t.Errorf("path = %q", got)
	}
	if result.Response.Summary != "2 revisions" {
		t.Errorf("summary = %q", result.Response.Summary)
	}
	if got := registerBreadcrumb(result, "latest"); got != "mailpiece post history show p1 latest" {
		t.Errorf("latest breadcrumb = %q", got)
	}
}

func TestPostHistoryShow(t *testing.T) {
	for _, id := range []string{"r1", "latest"} {
		mock, result := mockTest(t)
		mock.WithGetData(map[string]any{"id": "r1", "title": "Before", "body_html": "<p>Before</p>"})

		assertExitCode(t, postHistoryShowCmd.RunE(postHistoryShowCmd, []string{"p1", id}), 0)

		if got := mock.GetCalls[0].Path; got != "/posts/p1/history/"+id+".json" {
			t.Errorf("path = %q", got)
		}
		if data := responseDataMap(t, result); data["title"] != "Before" || data["body_html"] != "<p>Before</p>" {
			t.Errorf("data = %#v", data)
		}
		resetTest()
	}
}
