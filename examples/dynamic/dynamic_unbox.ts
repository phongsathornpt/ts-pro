function dynamicBoolScore(value: boolean): number {
  if (value) {
    return 42;
  }
  return 0;
}

let dynamicUnboxNumber: any = 41;
let dynamicUnboxNumberNative: number = dynamicUnboxNumber;
console.log(dynamicUnboxNumberNative + 1);

let dynamicUnboxString: any = "unboxed";
let dynamicUnboxStringNative: string = dynamicUnboxString;
console.log(dynamicUnboxStringNative);

let dynamicUnboxBoolean: any = true;
let dynamicUnboxBooleanNative: boolean = dynamicUnboxBoolean;
console.log(dynamicBoolScore(dynamicUnboxBooleanNative));

let dynamicUnboxArray: any = [40, 2];
let dynamicUnboxArrayNative: number[] = dynamicUnboxArray;
console.log(dynamicUnboxArrayNative[0]! + dynamicUnboxArrayNative[1]!);

type DynamicUnboxObject = { value: number };
let dynamicUnboxObject: any = { value: 42 };
let dynamicUnboxObjectNative: DynamicUnboxObject = dynamicUnboxObject;
console.log(dynamicUnboxObjectNative.value);

function dynamicUnboxFunctionTarget(value: number): number {
  return value + 1;
}
let dynamicUnboxFunction: any = dynamicUnboxFunctionTarget;
let dynamicUnboxFunctionNative: (value: number) => number = dynamicUnboxFunction;
console.log(dynamicUnboxFunctionNative(41));
