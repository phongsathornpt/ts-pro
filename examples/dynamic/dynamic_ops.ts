let dynamicOpsSix: any = "6";
let dynamicOpsSeven: any = 7;

console.log(dynamicOpsSix - 1);
console.log(dynamicOpsSix * dynamicOpsSeven);
console.log(84 / dynamicOpsSeven);

let dynamicOpsLess: any = dynamicOpsSix < dynamicOpsSeven;
let dynamicOpsTen: any = "10";
let dynamicOpsTwo: any = "2";
let dynamicOpsStringLess: any = dynamicOpsTen < dynamicOpsTwo;
let dynamicOpsLessEqual: any = dynamicOpsSix <= 6;
let dynamicOpsGreater: any = dynamicOpsSeven > dynamicOpsSix;
let dynamicOpsGreaterEqual: any = dynamicOpsSeven >= 7;
console.log(dynamicOpsLess);
console.log(dynamicOpsStringLess);
console.log(dynamicOpsLessEqual);
console.log(dynamicOpsGreater);
console.log(dynamicOpsGreaterEqual);

let dynamicOpsLoose: any = dynamicOpsSix == 6;
let dynamicOpsStrict: any = dynamicOpsSix === 6;
let dynamicOpsLooseNot: any = dynamicOpsSix != 6;
let dynamicOpsStrictNot: any = dynamicOpsSix !== 6;
console.log(dynamicOpsLoose);
console.log(dynamicOpsStrict);
console.log(dynamicOpsLooseNot);
console.log(dynamicOpsStrictNot);

let dynamicOpsNullValue: any = null;
let dynamicOpsUndefinedValue: any = undefined;
let dynamicOpsNullLoose: any = dynamicOpsNullValue == dynamicOpsUndefinedValue;
let dynamicOpsNullStrict: any = dynamicOpsNullValue === dynamicOpsUndefinedValue;
console.log(dynamicOpsNullLoose);
console.log(dynamicOpsNullStrict);

const dynamicOpsShared = { value: 42 };
let dynamicOpsObjectA: any = dynamicOpsShared;
let dynamicOpsObjectB: any = dynamicOpsShared;
let dynamicOpsSameObject: any = dynamicOpsObjectA === dynamicOpsObjectB;
console.log(dynamicOpsSameObject);
