package main

import (
	"strconv"
	"time"
)

// SnowflakeTimestamp returns the creation time of a Snowflake ID relative to the creation of Discord.
func SnowflakeTimestamp(ID string) (t time.Time, err error) {
	i, err := strconv.ParseInt(ID, 10, 64)
	if err != nil {
		return
	}
	timestamp := (i >> 22) + 1420070400000
	t = time.Unix(0, timestamp*1000000)
	return
}

// Checks if string is in list
func belongsToList(lookup string, list []string) bool {
	for _, val := range list {
		if val == lookup {
			return true
		}
	}
	return false
}
