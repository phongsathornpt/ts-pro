interface DynamicCallBox {
  value: number;
}

function dynamicAddOne(value: number): number {
  return value + 1;
}

function dynamicReadBox(box: DynamicCallBox): number {
  return box.value;
}

function runDynamicCalls(): void {
  const addAny: any = dynamicAddOne;
  const readAny: any = dynamicReadBox;
  const box: DynamicCallBox = { value: 42 };
  const offset: number = 5;
  const captured: any = (value: number): number => value + offset;

  console.log(addAny(41));
  console.log(readAny(box));
  console.log(captured(37));
}

runDynamicCalls();
