interface Point {
  x: number;
  y: number;
}

function sumPoint(point: Point): number {
  return point.x + point.y;
}

console.log(sumPoint({ y: 4, x: 3 }));
