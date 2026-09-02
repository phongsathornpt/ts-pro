function identityAny(value: any): any {
  return value;
}

let truth: any = true;
let two: any = 2;
let dynamicBoolPrefix: any = "bool=";

console.log(truth);
console.log(truth + two);
console.log(dynamicBoolPrefix + truth);
console.log(identityAny(false));
