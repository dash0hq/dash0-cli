package output

import "fmt"

// Pluralize returns "1 thing" or "N things". It forms the plural by adding "s", which holds for every noun the
// CLI counts.
func Pluralize(count int, singular string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, singular)
	}
	return fmt.Sprintf("%d %ss", count, singular)
}
