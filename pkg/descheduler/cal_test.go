package descheduler

import (
	"fmt"
	"testing"
	"time"
)

func TestDe(t *testing.T) {
	oldSchedulerInfo := map[string]int64{
		"c1": 1,
		"c2": time.Now().Unix() - 2*60,
		"c3": 5,
		"c4": 0,
	}

	all := []string{"c1", "c2", "c3", "c6"}
	//all = []string{"c6", "c7"}
	newSchedulerInfo, newChooseCluster, found := procSchedule(all, encodeSchedule(oldSchedulerInfo), []string{"c1"})

	fmt.Println(newSchedulerInfo)
	fmt.Println(newChooseCluster)
	fmt.Println(found)
}
