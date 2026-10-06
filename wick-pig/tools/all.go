package tools

// All is every file tool this package serves, in loadout order.
func All() []Tool {
	return []Tool{readTool{}, editTool{}, writeTool{}, bashTool{}, findTool{}, websearchTool{}, grepTool{}, gitTool{}, checkTool{}, backgroundTool{}, ctxSearchTool{}}
}

// Named returns the tools called names, in the order given. Unknown names are
// skipped: a type that lists a tool the wick host serves (spawn, submit) is not
// an error here.
func Named(names ...string) []Tool {
	byName := map[string]Tool{}
	for _, t := range All() {
		byName[t.Name()] = t
	}
	var out []Tool
	for _, n := range names {
		if t, ok := byName[n]; ok {
			out = append(out, t)
		}
	}
	return out
}
