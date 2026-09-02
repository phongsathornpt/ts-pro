function arrayAddOne(x: number): number { return x + 1; }
const arrayOffset = 5;
const arrayAddOffset = (x: number): number => x + arrayOffset;
const functionArray: Array<(x: number) => number> = [arrayAddOne, arrayAddOffset];
console.log(functionArray.length);
console.log(functionArray[0]!(3));
for (let i = 0; i < 20000; i++) {
  const churn = "g" + "c";
}
console.log(functionArray[1]!(7));
