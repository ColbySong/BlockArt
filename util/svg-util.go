package util

import (
	"math"
	"strconv"
	"strings"
	"unicode"

	"../blockartlib"
)

var validOperations = []rune{'M', 'm', 'L', 'l', 'H', 'h', 'V', 'v', 'Z', 'z'}

type SVGPathCoordinates struct {
	XCords []int
	YCords []int
}

type Point struct {
	xCord int
	yCord int
}

// Validate SVG Path String ("M 0 0 L 0 5")
// Returns the following errors:
// - ShapeSVGStringTooLongError
// - InvalidShapeSvgStringError
func ValidateShapeSVGString(shapeSvgString string) (bool, error) {
	if len(shapeSvgString) > 128 {
		return false, blockartlib.ShapeSvgStringTooLongError(shapeSvgString)
	}

	for _, char := range shapeSvgString {
		if unicode.IsLetter(char) {
			if !isOperationValid(char) {
				return false, blockartlib.InvalidShapeSvgStringError(shapeSvgString)
			}
		} else {
			if !unicode.IsSpace(char) && !unicode.IsDigit(char) {
				return false, blockartlib.InvalidShapeSvgStringError(shapeSvgString)
			}
		}
	}

	if _, err := ConvertPathToPoints(shapeSvgString); err != nil {
		return false, blockartlib.InvalidShapeSvgStringError(shapeSvgString)
	}

	return true, nil
}

// Returns the ink required to draw a svg path
// @param isTransparent - true if svg command issued is transparent, false if non-transparent
// @param isClosed - true if svg path is a closed shape
func CalculateInkRequired(svgPath SVGPathCoordinates, isTransparent bool, isClosed bool) uint32 {
	var ink int
	numPoints := len(svgPath.XCords)
	// if SVG is transparent, then the ink required is just the length of all the path
	// if SVG is non-transparent, then ink required is the area
	if isTransparent {
		for i := 0; i < len(svgPath.XCords)-1; i++ {
			xSqrd := math.Pow(float64(svgPath.XCords[i+1]-svgPath.XCords[i]), 2)
			ySqrd := math.Pow(float64(svgPath.YCords[i+1]-svgPath.YCords[i]), 2)
			length := math.Sqrt(xSqrd + ySqrd)
			ink += int(length)
		}

		// if the shape is closed, we need to add an additional line from the last point to the first point
		if isClosed {
			xSqrd := math.Pow(float64(svgPath.XCords[len(svgPath.XCords)-1]-svgPath.XCords[0]), 2)
			ySqrd := math.Pow(float64(svgPath.YCords[len(svgPath.YCords)-1]-svgPath.YCords[0]), 2)
			length := math.Sqrt(xSqrd + ySqrd)
			ink += int(length)
		}
	} else {
		// calculate area of a polygon
		j := numPoints - 1
		for i := 0; i < numPoints; i++ {
			ink += int((svgPath.XCords[j] + svgPath.XCords[i]) * (svgPath.YCords[j] - svgPath.YCords[i]))
			j = i
		}
		ink = int(ink / 2)

		// if the path was constructed counter clock wise, the area would be negative
		// we need to convert it to positive before casting it as an uint32
		if ink < 0 {
			ink = -ink
		}
	}
	return uint32(ink)
}

// Returns true if the given svg path goes out of the bounds of the canvas
func CheckOutOfBounds(svgPath SVGPathCoordinates, canvasXMax uint32, canvasYMax uint32) bool {
	minX, maxX := minMax(svgPath.XCords)
	minY, maxY := minMax(svgPath.YCords)

	if minX < 0 || minY < 0 {
		return true
	} else if uint32(maxX) > canvasXMax || uint32(maxY) > canvasYMax {
		return true
	}

	return false
}

// Return true if two svgPath overlaps
func CheckOverlap(svgPathOne SVGPathCoordinates, svgPathTwo SVGPathCoordinates) bool {
	for i := 0; i < len(svgPathOne.XCords)-1; i++ {
		p1 := Point{xCord: svgPathOne.XCords[i], yCord: svgPathOne.YCords[i]}
		p2 := Point{xCord: svgPathOne.XCords[i+1], yCord: svgPathOne.YCords[i+1]}

		for j := 0; j < len(svgPathTwo.XCords)-1; j++ {
			p3 := Point{xCord: svgPathTwo.XCords[j], yCord: svgPathTwo.YCords[j]}
			p4 := Point{xCord: svgPathTwo.XCords[j+1], yCord: svgPathTwo.YCords[j+1]}
			if intersect(p1, p2, p3, p4) {
				return true
			}
		}
	}

	return false
}

// Convert a SVG path string to list of x and y points
func ConvertPathToPoints(shapeSvgString string) (SVGPathCoordinates, error) {
	var xCords []int
	var yCords []int
	splitStrings := strings.Split(shapeSvgString, " ")
	for i, str := range splitStrings {
		switch str {
		case "M", "L":
			absXCord, xerr := strconv.Atoi(splitStrings[i+1])
			absYCord, yerr := strconv.Atoi(splitStrings[i+2])
			if xerr != nil || yerr != nil {
				return SVGPathCoordinates{}, blockartlib.InvalidShapeSvgStringError(shapeSvgString)
			}
			xCords = append(xCords, absXCord)
			yCords = append(yCords, absYCord)
		case "m", "l":
			relXCord, xerr := strconv.Atoi(splitStrings[i+1])
			relYCord, yerr := strconv.Atoi(splitStrings[i+2])
			if xerr != nil || yerr != nil {
				return SVGPathCoordinates{}, blockartlib.InvalidShapeSvgStringError(shapeSvgString)
			}
			xCords = append(xCords, xCords[len(xCords)-1]+relXCord)
			yCords = append(yCords, yCords[len(yCords)-1]+relYCord)
		case "V":
			absYCord, yerr := strconv.Atoi(splitStrings[i+1])
			if yerr != nil {
				return SVGPathCoordinates{}, blockartlib.InvalidShapeSvgStringError(shapeSvgString)
			}
			xCords = append(xCords, xCords[len(xCords)-1])
			yCords = append(yCords, absYCord)
		case "v":
			relYCord, yerr := strconv.Atoi(splitStrings[i+1])
			if yerr != nil {
				return SVGPathCoordinates{}, blockartlib.InvalidShapeSvgStringError(shapeSvgString)
			}
			xCords = append(xCords, xCords[len(xCords)-1])
			yCords = append(yCords, yCords[len(yCords)-1]+relYCord)
		case "H":
			absXCord, xerr := strconv.Atoi(splitStrings[i+1])
			if xerr != nil {
				return SVGPathCoordinates{}, blockartlib.InvalidShapeSvgStringError(shapeSvgString)
			}
			xCords = append(xCords, absXCord)
			yCords = append(yCords, yCords[len(yCords)-1])
		case "h":
			relXCord, xerr := strconv.Atoi(splitStrings[i+1])
			if xerr != nil {
				return SVGPathCoordinates{}, blockartlib.InvalidShapeSvgStringError(shapeSvgString)
			}
			xCords = append(xCords, xCords[len(xCords)-1]+relXCord)
			yCords = append(yCords, yCords[len(yCords)-1])
		}
	}
	return SVGPathCoordinates{XCords: xCords, YCords: yCords}, nil
}

// Return true if line segments p1p2 and p3p4 intersect
func intersect(p1, p2, p3, p4 Point) bool {
	return (ccw(p1, p3, p4) != ccw(p2, p3, p4)) && (ccw(p1, p2, p3) != ccw(p1, p2, p4))
}

func ccw(p1, p2, p3 Point) bool {
	return (p3.yCord-p1.yCord)*(p2.xCord-p1.xCord) > (p2.yCord-p1.yCord)*(p3.xCord-p1.xCord)
}

func isOperationValid(op rune) bool {
	for _, vOp := range validOperations {
		if vOp == op {
			return true
		}
	}
	return false
}

func minMax(array []int) (int, int) {
	var max int = array[0]
	var min int = array[0]
	for _, value := range array {
		if max < value {
			max = value
		}
		if min > value {
			min = value
		}
	}
	return min, max
}
