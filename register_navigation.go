package main

// spatialNeighbor chooses a pane in the requested screen direction within the
// same tab/window. At an edge the selection stays put, like mux focus movement.
func spatialNeighbor(panes []Pane, selected int, direction byte) int {
	if selected < 0 || selected >= len(panes) {
		return selected
	}
	current := panes[selected]
	best, bestGap, bestAxis, bestCross := selected, int(^uint(0)>>1), int(^uint(0)>>1), int(^uint(0)>>1)
	if current.Width <= 0 || current.Height <= 0 {
		return orderedNeighbor(panes, selected, direction)
	}
	for i, p := range panes {
		if i == selected || p.TabID != current.TabID || p.Width <= 0 || p.Height <= 0 {
			continue
		}
		cx, cy := 2*current.X+current.Width, 2*current.Y+current.Height
		px, py := 2*p.X+p.Width, 2*p.Y+p.Height
		var axis, cross, gap int
		switch direction {
		case 'L':
			if px >= cx {
				continue
			}
			axis = max(0, current.X-(p.X+p.Width))
			gap = intervalGap(current.Y, current.Y+current.Height, p.Y, p.Y+p.Height)
			cross = abs(cy - py)
		case 'R':
			if px <= cx {
				continue
			}
			axis = max(0, p.X-(current.X+current.Width))
			gap = intervalGap(current.Y, current.Y+current.Height, p.Y, p.Y+p.Height)
			cross = abs(cy - py)
		case 'U':
			if py >= cy {
				continue
			}
			axis = max(0, current.Y-(p.Y+p.Height))
			gap = intervalGap(current.X, current.X+current.Width, p.X, p.X+p.Width)
			cross = abs(cx - px)
		case 'D':
			if py <= cy {
				continue
			}
			axis = max(0, p.Y-(current.Y+current.Height))
			gap = intervalGap(current.X, current.X+current.Width, p.X, p.X+p.Width)
			cross = abs(cx - px)
		default:
			return selected
		}
		if gap < bestGap || (gap == bestGap && (axis < bestAxis || (axis == bestAxis && cross < bestCross))) {
			best, bestGap, bestAxis, bestCross = i, gap, axis, cross
		}
	}
	return best
}

func intervalGap(a0, a1, b0, b1 int) int {
	if a1 < b0 {
		return b0 - a1
	}
	if b1 < a0 {
		return a0 - b1
	}
	return 0
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func orderedNeighbor(panes []Pane, selected int, direction byte) int {
	step := 1
	if direction == 'U' || direction == 'L' {
		step = -1
	}
	for i := selected + step; i >= 0 && i < len(panes); i += step {
		if panes[i].TabID == panes[selected].TabID {
			return i
		}
	}
	return selected
}

// cycleTab is explicit because physical directions cannot cross tabs/windows.
func cycleTab(panes []Pane, selected, step int) int {
	var tabs []string
	for _, p := range panes {
		found := false
		for _, tab := range tabs {
			if tab == p.TabID {
				found = true
				break
			}
		}
		if !found {
			tabs = append(tabs, p.TabID)
		}
	}
	if len(tabs) < 2 {
		return selected
	}
	current := 0
	for i, tab := range tabs {
		if tab == panes[selected].TabID {
			current = i
			break
		}
	}
	target := tabs[(current+step+len(tabs))%len(tabs)]
	for i, p := range panes {
		if p.TabID == target {
			return i
		}
	}
	return selected
}
