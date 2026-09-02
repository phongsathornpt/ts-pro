type ObjectArrayPoint = { x: number; y: number };
const a: ObjectArrayPoint = { x: 1, y: 2 };
const b: ObjectArrayPoint = { x: 3, y: 4 };
const points: ObjectArrayPoint[] = [a, b];
console.log(points.length);
console.log(points[0]!.x);
points[1] = { x: 7, y: 8 };
for (let i = 0; i < 20000; i++) {
  const churn = "g" + "c";
}
console.log(points[1]!.x);
