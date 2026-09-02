type DynamicUnion = number | string | boolean | null | undefined;

function echoUnion(value: DynamicUnion): DynamicUnion {
  return value;
}

console.log(echoUnion(42));
console.log(echoUnion("union"));
console.log(echoUnion(true));
console.log(echoUnion(null));
console.log(echoUnion(undefined));

type ReferenceUnion = { value: number } | number[] | ((value: number) => number);

function consumeReferenceUnion(value: ReferenceUnion): number {
  return 1;
}

function incrementUnion(value: number): number {
  return value + 1;
}

console.log(consumeReferenceUnion({ value: 42 }));
console.log(consumeReferenceUnion([1, 2]));
console.log(consumeReferenceUnion(incrementUnion));
