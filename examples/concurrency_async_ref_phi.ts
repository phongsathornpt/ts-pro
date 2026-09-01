async function asyncStringBranch(flag: boolean): Promise<string> {
  let value = "start";
  if (flag) {
    value = "left";
  } else {
    value = "right";
  }
  sleep(1);
  return value + ":done";
}

async function asyncStringLoop(limit: number): Promise<string> {
  let value = "x";
  let i = 0;
  while (i < limit) {
    value = value + "y";
    sleep(1);
    i++;
  }
  return value;
}

console.log(join(asyncStringBranch(true)));
console.log(join(asyncStringBranch(false)));
console.log(join(asyncStringLoop(3)));

async function asyncObjectBranch(flag: boolean): Promise<{ value: number }> {
  let value = { value: 1 };
  if (flag) {
    value = { value: 42 };
  } else {
    value = { value: 7 };
  }
  sleep(1);
  return value;
}

async function asyncAnyBranch(flag: boolean): Promise<any> {
  let value: any = "start";
  if (flag) {
    value = "dynamic-left";
  } else {
    value = "dynamic-right";
  }
  sleep(1);
  return value;
}

console.log(join(asyncObjectBranch(true)).value);
console.log(join(asyncObjectBranch(false)).value);
console.log(join(asyncAnyBranch(true)));
console.log(join(asyncAnyBranch(false)));
