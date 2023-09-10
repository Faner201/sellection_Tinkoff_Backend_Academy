package secondtask

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

func main() {
	var count int
	reader := bufio.NewReader(os.Stdin)
	fmt.Scan(&count)

	var lines []string
	var number string
	var countTwo int
	for i := 0; i < count; i++ {
		line, _ := reader.ReadString('\n')
		length := utf8.RuneCountInString(line)
		if length > 2 {
			number = line[2:]
			lines = append(lines, number)
		}
		if strings.Contains(line, "2") {
			countTwo++
			for i = 0; i < 1*countTwo; i++ {
				lines = append(lines, number)
			}
		}
		if strings.Contains(line, "3") {
			fmt.Println(lines[0])
			lines = append(lines[:0], lines[1:]...)
		}
	}
}
