package test

import (
	"../util"
	"testing"
	"reflect"
)

// go test svg_util_test.go

// "M 150 150 V 50 H 20 Z", area = 6500, perimeter = 394.01
var rightAngleTriangle = util.SVGPathCoordinates{ XCords: []int{150, 150, 20}, YCords: []int{150, 50, 50} }

// "M 150 150 v 50 L 100 100 Z"
var weirdAssTriangle = util.SVGPathCoordinates{ XCords: []int{150, 150, 100}, YCords: []int{150, 200, 100} }

// "M 100 100 L 100 200 h 100 v -100 Z"
var regularAssSquare = util.SVGPathCoordinates{ XCords: []int{100, 100, 200, 200}, YCords: []int{100, 200, 200, 100} }

// "M 50 50 L 50 150 h 100 v -100 Z"
var squareThatOverlapsRegularAssSquare = util.SVGPathCoordinates{ XCords: []int{50, 50, 150, 150}, YCords: []int{50, 150, 150, 50} }

// "M 50 50 L 50 80 h 30 v -30 Z"
var squareThatDoesntOverLap = util.SVGPathCoordinates{ XCords: []int{50, 50, 80, 80}, YCords: []int{50, 80, 80, 50} }

// "M 100 100 L 300 100 L 300 200 L 100 200 Z"
var regularAssRectangle = util.SVGPathCoordinates{ XCords: []int{100, 300, 300, 100}, YCords: []int{100, 100, 200, 200} }

// "M 100 100 L 300 100 L 400 200 L 200 200 Z"
var regularAssPolygon = util.SVGPathCoordinates{ XCords: []int{100, 300, 400, 200}, YCords: []int{100, 100, 200, 200} }

func TestValidateShapeSVGString(t *testing.T) {
	if _, err := util.ValidateShapeSVGString("M 0 0 L 0 5"); err != nil {
		t.Errorf("SVG String is valid but got error: %s", err)
	}

	// Q operation not supported
	if isValid, _ := util.ValidateShapeSVGString("M 0 0 Q 0 5"); isValid {
		t.Error("SVG String is invalid but got true")
	}

	// no comma seperators allowed
	if isValid, _ := util.ValidateShapeSVGString("M 0, 0 L 0, 5"); isValid {
		t.Error("SVG String is invalid but got true")
	}

	// Missing arguments to L command
	if isValid, _ := util.ValidateShapeSVGString("M 0 0 L 0 H 10"); isValid {
		t.Error("SVG String is invalid but got true")
	}

}

func TestConvertPathToPoints(t *testing.T) {
	if svgPath, _ := util.ConvertPathToPoints("M 150 150 V 50 H 20 Z"); !reflect.DeepEqual(svgPath, rightAngleTriangle) {
		t.Errorf("Expected: %+v, but got %+v", rightAngleTriangle, svgPath)
	}

	if svgPath, _ := util.ConvertPathToPoints("M 150 150 v 50 L 100 100 Z"); !reflect.DeepEqual(svgPath, weirdAssTriangle) {
		t.Errorf("Expected: %+v, but got %+v", weirdAssTriangle, svgPath)
	}

	if svgPath, _ := util.ConvertPathToPoints("M 100 100 L 100 200 h 100 v -100 Z"); !reflect.DeepEqual(svgPath, regularAssSquare) {
		t.Errorf("Expected: %+v, but got %+v", regularAssSquare, svgPath)
	}

	if svgPath, _ := util.ConvertPathToPoints("M 50 50 L 50 150 h 100 v -100 Z"); !reflect.DeepEqual(svgPath, squareThatOverlapsRegularAssSquare) {
		t.Errorf("Expected: %+v, but got %+v", squareThatOverlapsRegularAssSquare, svgPath)
	}

	if svgPath, _ := util.ConvertPathToPoints("M 50 50 L 50 80 h 30 v -30 Z"); !reflect.DeepEqual(svgPath, squareThatDoesntOverLap) {
		t.Errorf("Expected: %+v, but got %+v",squareThatDoesntOverLap, svgPath)
	}

	if svgPath, _ := util.ConvertPathToPoints("M 100 100 L 300 100 L 300 200 L 100 200 Z"); !reflect.DeepEqual(svgPath, regularAssRectangle) {
		t.Errorf("Expected: %+v, but got %+v", regularAssRectangle, svgPath)
	}

	if svgPath, _ := util.ConvertPathToPoints("M 100 100 L 300 100 L 400 200 L 200 200 Z"); !reflect.DeepEqual(svgPath, regularAssPolygon) {
		t.Errorf("Expected: %+v, but got %+v", regularAssPolygon, svgPath)
	}
}

func TestCalculateInkRequired(t *testing.T) {
	// Basic right angle triangle with 300, 400, 500 side length
	rightAngleTriangle := util.SVGPathCoordinates{ XCords: []int{150, 150, 550}, YCords: []int{150, 450, 450}}
	if ink := util.CalculateInkRequired(rightAngleTriangle, true, true); ink != uint32(1200) {
		t.Errorf("Expected ink: 1200, but got %d", ink)
	}

	if ink := util.CalculateInkRequired(rightAngleTriangle, true, false); ink != uint32(700) {
		t.Errorf("Expected ink: 700, but got %d", ink)
	}

	if ink := util.CalculateInkRequired(rightAngleTriangle, false, false); ink != uint32(60000) {
		t.Errorf("Expected ink: 60000, but got %d", ink)
	}

	// Square and Rectangle ink
	if ink := util.CalculateInkRequired(regularAssSquare, false, false); ink != uint32(10000) {
		t.Errorf("Expected ink: 10000, but got %d", ink)
	}

	if ink := util.CalculateInkRequired(regularAssRectangle, false, false); ink != uint32(20000) {
		t.Errorf("Expected ink: 20000, but got %d", ink)
	}

	if ink := util.CalculateInkRequired(regularAssRectangle, true, false); ink != uint32(500) {
		t.Errorf("Expected ink: 500, but got %d", ink)
	}

	if ink := util.CalculateInkRequired(regularAssRectangle, true, true); ink != uint32(600) {
		t.Errorf("Expected ink: 600, but got %d", ink)
	}

	// polygon
	if ink := util.CalculateInkRequired(regularAssPolygon, false, false); ink != uint32(20000) {
		t.Errorf("Expected ink: 20000, but got %d", ink)
	}


}

func TestCheckOutOfBounds(t *testing.T) {
	if err := util.CheckOutOfBounds(rightAngleTriangle, 1000, 1000); err != nil {
		t.Error("Path are within bound but got out of bounds")
	}

	if err := util.CheckOutOfBounds(rightAngleTriangle, 100, 100); err == nil {
		t.Error("Path are out of bounds but got within bounds ")
	}
}

func TestCheckOverLap(t *testing.T) {
	simpleLine := util.SVGPathCoordinates{ XCords: []int{100, 150}, YCords: []int{100, 150} }
	intersectsSimpleLine :=  util.SVGPathCoordinates{ XCords: []int{150, 50}, YCords: []int{120, 120} }
	noIntersect := util.SVGPathCoordinates{ XCords: []int{150, 50}, YCords: []int{90, 90} }

	// Two lines that intersect
	if err := util.CheckOverlap(simpleLine, intersectsSimpleLine); err == nil {
		t.Error("The two path over laps but got that they don't")
	}

	// Two lines that don't intersect
	if err := util.CheckOverlap(simpleLine, noIntersect); err != nil {
		t.Error("The two path DO NOT over laps but got that they do")
	}

	// Two squares that overlap
	if err := util.CheckOverlap(regularAssSquare, squareThatOverlapsRegularAssSquare); err == nil {
		t.Error("The two path over laps but got that they don't")
	}

	// Two squares that don't overlap
	if err := util.CheckOverlap(regularAssSquare, squareThatDoesntOverLap); err != nil {
		t.Error("The two path DO NOT over laps but got that they do")
	}


}
