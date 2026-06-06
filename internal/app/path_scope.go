package app

import "strings"

func pathGlobMatches(pattern string, repoPath string) bool {
	pattern = normalizePathGlob(pattern)
	repoPath = normalizeRepoPathForGlob(repoPath)
	if pattern == "" || repoPath == "" {
		return false
	}
	return pathGlobSegmentsMatch(strings.Split(pattern, "/"), strings.Split(repoPath, "/"))
}

func pathGlobMatchesAny(patterns []string, repoPath string) bool {
	for _, pattern := range patterns {
		if pathGlobMatches(pattern, repoPath) {
			return true
		}
	}
	return false
}

func normalizePathGlob(pattern string) string {
	pattern = strings.TrimSpace(filepathSlash(pattern))
	pattern = strings.TrimPrefix(pattern, "./")
	return strings.Trim(pattern, "/")
}

func normalizeRepoPathForGlob(repoPath string) string {
	repoPath = strings.TrimSpace(filepathSlash(repoPath))
	repoPath = strings.TrimPrefix(repoPath, "./")
	return strings.Trim(repoPath, "/")
}

func filepathSlash(value string) string {
	return strings.ReplaceAll(value, "\\", "/")
}

func pathGlobSegmentsMatch(patternSegments []string, pathSegments []string) bool {
	var match func(int, int) bool
	match = func(patternIndex int, pathIndex int) bool {
		if patternIndex == len(patternSegments) {
			return pathIndex == len(pathSegments)
		}

		patternSegment := patternSegments[patternIndex]
		if patternSegment == "**" {
			for patternIndex+1 < len(patternSegments) && patternSegments[patternIndex+1] == "**" {
				patternIndex++
			}
			if patternIndex == len(patternSegments)-1 {
				return true
			}
			for nextPathIndex := pathIndex; nextPathIndex <= len(pathSegments); nextPathIndex++ {
				if match(patternIndex+1, nextPathIndex) {
					return true
				}
			}
			return false
		}

		if pathIndex == len(pathSegments) {
			return false
		}
		if !pathGlobSegmentMatches(patternSegment, pathSegments[pathIndex]) {
			return false
		}
		return match(patternIndex+1, pathIndex+1)
	}
	return match(0, 0)
}

func pathGlobSegmentMatches(pattern string, segment string) bool {
	previous := make([]bool, len(segment)+1)
	previous[0] = true
	for patternIndex := 0; patternIndex < len(pattern); patternIndex++ {
		current := make([]bool, len(segment)+1)
		if pattern[patternIndex] == '*' {
			current[0] = previous[0]
			for segmentIndex := 1; segmentIndex <= len(segment); segmentIndex++ {
				current[segmentIndex] = previous[segmentIndex] || current[segmentIndex-1]
			}
		} else {
			for segmentIndex := 1; segmentIndex <= len(segment); segmentIndex++ {
				current[segmentIndex] = previous[segmentIndex-1] && pattern[patternIndex] == segment[segmentIndex-1]
			}
		}
		previous = current
	}
	return previous[len(segment)]
}
