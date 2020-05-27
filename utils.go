package main

import (
	"strconv"
	"sync"
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
func belongsToList(list []string, lookup string) bool {
	for _, val := range list {
		if val == lookup {
			return true
		}
	}
	return false
}

func extractIDs(list []*interface{}) (ids []string) {
	for _, v := range list {
		a := *v
		ids = append(ids, a.(struct{ ID string }).ID)
	}
	return
}

// LockSet allows for a python-like set which allows for concurrent use
type LockSet struct {
	sync.RWMutex
	Values []string `json:"values" msgpack:"values"`
}

// Contains returns a boolean if the set contains a specific value
func (ls *LockSet) Contains(_val string) (contains bool) {
	ls.RLock()
	defer ls.RUnlock()

	for _, val := range ls.Values {
		if val == _val {
			contains = true
			break
		}
	}

	return
}

// Get returns the value of the LockSet
func (ls *LockSet) Get() (values []string) {
	ls.RLock()
	defer ls.RUnlock()

	return ls.Values
}

// Remove removes a value from the LockSet
func (ls *LockSet) Remove(_val string) (values []string, change bool) {
	ls.Lock()
	defer ls.Unlock()

	newVals := make([]string, 0)
	for _, val := range ls.Values {
		if val != _val {
			newVals = append(newVals, val)
		} else {
			change = true
		}
	}

	return ls.Values, change
}

// Add adds a value to the LockSet
func (ls *LockSet) Add(_val string) (values []string, change bool) {
	ls.Lock()
	defer ls.Unlock()

	alreadyContains := false
	for _, val := range ls.Values {
		if val == _val {
			alreadyContains = true
			break
		}
	}

	if !alreadyContains {
		ls.Values = append(ls.Values, _val)
		change = true
	}

	return ls.Values, change
}
