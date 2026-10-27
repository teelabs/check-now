package semver

import (
	"fmt"
	"strconv"
	"strings"
)

type Version struct {
	Major, Minor, Patch int
	Pre                 string
}

func Parse(s string) (Version, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	parts := strings.SplitN(s, "-", 2)
	nums := strings.Split(parts[0], ".")
	if len(nums) < 2 || len(nums) > 3 {
		return Version{}, fmt.Errorf("invalid version %q", s)
	}
	n := make([]int, 3)
	for i := range nums {
		v, e := strconv.Atoi(nums[i])
		if e != nil || v < 0 {
			return Version{}, fmt.Errorf("invalid version %q", s)
		}
		n[i] = v
	}
	var pre string
	if len(parts) == 2 {
		pre = parts[1]
	}
	return Version{n[0], n[1], n[2], pre}, nil
}
func (v Version) Compare(o Version) int {
	for _, p := range [][2]int{{v.Major, o.Major}, {v.Minor, o.Minor}, {v.Patch, o.Patch}} {
		if p[0] != p[1] {
			if p[0] < p[1] {
				return -1
			}
			return 1
		}
	}
	if v.Pre == o.Pre {
		return 0
	}
	if v.Pre == "" {
		return 1
	}
	if o.Pre == "" {
		return -1
	}
	if v.Pre < o.Pre {
		return -1
	}
	return 1
}
func Latest(current, target string) (bool, error) {
	a, e := Parse(current)
	if e != nil {
		return false, e
	}
	b, e := Parse(target)
	if e != nil {
		return false, e
	}
	return b.Compare(a) > 0, nil
}
func SameMajor(current, target string) (bool, error) {
	a, e := Parse(current)
	if e != nil {
		return false, e
	}
	b, e := Parse(target)
	if e != nil {
		return false, e
	}
	return a.Major == b.Major && b.Compare(a) > 0, nil
}
