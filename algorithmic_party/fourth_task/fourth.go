package fourthtask

import (
	"fmt"
	"strconv"
	"unicode/utf8"
)

func main() {
	var number, a, b int

	fmt.Scan(&number)

	var report []int

	for i := 0; i < number; i++ {
		fmt.Scan(&a, &b)
		count := 1
		for j := a; j < b+1; j++ {
			count *= j
		}

		for count > 9 {
			stringNumber := strconv.Itoa(count)
			sum := 0
			numberResidue := count
			for j := 0; j < utf8.RuneCountInString(stringNumber); j++ {
				sum += numberResidue % 10
				numberResidue = numberResidue / 10
			}
			fmt.Println(sum)
			count = sum
		}
		report = append(report, count)
	}

	for _, num := range report {
		fmt.Println(num)
	}
}
