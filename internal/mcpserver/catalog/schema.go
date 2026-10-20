package catalog

// Schema shorthands for the hand-written domains_<domain>.go files.

func str(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func strEnum(desc string, values ...any) map[string]any {
	return map[string]any{"type": "string", "description": desc, "enum": values}
}

func integer(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}

func boolean(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

// object is a body schema: the properties a write accepts, and which of them
// it requires.
func object(properties map[string]any, required ...any) map[string]any {
	schema := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func pathParam(name, desc string) Param {
	return Param{Name: name, In: "path", Required: true, Description: desc, Schema: str(desc)}
}

func queryParam(name, desc string, schema map[string]any) Param {
	return Param{Name: name, In: "query", Description: desc, Schema: schema}
}

func pageParam() Param {
	return queryParam("page", "Page number; responses include next_page when more results exist.", integer("Page number"))
}

// cannotBeUndone ends the summary of a destructive write: one that deletes,
// or mails, or otherwise cannot be taken back.
const cannotBeUndone = "This cannot be undone."
