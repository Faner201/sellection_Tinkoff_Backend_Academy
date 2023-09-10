package firsttask

import (
	"fmt"
	"math"
)

func main() {
	var number int
	count := 1.0
	fmt.Scan(&number)

	squares := []int{0}
	var countPow []float64
	for i := 0; i < number; i++ {
		count += 2.0
		countPow = append(countPow, math.Pow(count, 2.0))
		sum := 1
		for j := 0; j < len(countPow); j++ {
			sum += int(countPow[j])
		}
		squares = append(squares, int(math.Pow(count, 3.0))-sum)
	}

	for i := 0; i < len(squares)-1; i++ {
		fmt.Println(squares[i])
	}
}
