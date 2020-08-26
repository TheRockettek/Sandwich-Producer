package gateway

// RediScripts contains all the custom redis scripts
type RediScripts struct{}

// ClearKeys allows for you to clear redis keys based off of a pattern.
// We really should not be doing it this way. It really should be
// scanning keys, finding guilds that would belong to this cluster
// through normal shardID bitwise calculations, iterating through
// guild members, removing their mutual and if it is empty, remove
// their user object, remove the guild members reguardless and lastly
// delete the emoji fields, role fields, channel fields and the guild
// entry. It could be possible to add an expiry on the objects
// themself but that would require to constantly update the expiry on
// objects as it could be that it expires too early so would need
// to renew it more often or too long that it never is removed.
// Reguardless, either way would require another routine to keep
// all entries constantly refreshed.
func (*RediScripts) ClearKeys(pattern string, m *Manager) (result int64, err error) {
	if _result, err := m.RedisClient.Eval(
		m.ctx,
		`local count, cursor = 0, "0"
		while true do
			local req = redis.call("SCAN", cursor, "MATCH", ARGV[1], "COUNT", ARGV[2], "TYPE", "string")
			if #req[2] > 0 then redis.call("DEL", unpack(req[2])) end
			count, cursor = count + #req[2], req[1]
			if cursor == "0" then break end
		end
		return count`,
		[]string{},
		pattern,
		64,
	).Result(); err == nil {
		result = _result.(int64)
	}
	return
}
