function identityNullishAny(value: any): any {
  return value;
}

let dynamicNullValue: any = null;
let dynamicUndefinedValue: any = undefined;
let dynamicNullishTwo: any = 2;
let dynamicNullishPrefix: any = "value=";

console.log(dynamicNullValue);
console.log(dynamicUndefinedValue);
console.log(dynamicNullValue + dynamicNullishTwo);
console.log(dynamicNullishPrefix + dynamicNullValue);
console.log(dynamicNullishPrefix + dynamicUndefinedValue);
console.log(dynamicUndefinedValue + dynamicNullishTwo);
console.log(identityNullishAny(null));
console.log(identityNullishAny(undefined));
