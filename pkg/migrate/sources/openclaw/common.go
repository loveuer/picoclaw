package openclaw

var migrateableFiles = []string{
	"AGENTS.md",
	"SOUL.md",
	"USER.md",
	"HEARTBEAT.md",
}

var migrateableDirs = []string{
	"memory",
	"skills",
}

var supportedChannels = map[string]bool{
	"telegram": true,
	"feishu":   true,
	"qq":       true,
	"dingtalk": true,
	"onebot":   true,
	"wecom":    true,
}
