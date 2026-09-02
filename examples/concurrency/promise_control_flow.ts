async function branchPromiseThen(): Promise<number> {
  let current: Promise<number> = Promise.resolve(1);
  if (1 < 2) {
    current = Promise.resolve(2);
  } else {
    current = Promise.resolve(3);
  }
  return await current;
}

async function branchPromiseElse(): Promise<number> {
  let current: Promise<number> = Promise.resolve(4);
  if (2 < 1) {
    current = Promise.resolve(5);
  } else {
    current = Promise.resolve(6);
  }
  return await current;
}

async function loopPromiseOwner(): Promise<number> {
  let current: Promise<number> = Promise.resolve(0);
  let index = 0;
  while (index < 3) {
    if (index === 1) {
      current = Promise.resolve(9);
    } else {
      current = Promise.resolve(index + 1);
    }
    index++;
  }
  return await current;
}

console.log(join(branchPromiseThen()));
console.log(join(branchPromiseElse()));
console.log(join(loopPromiseOwner()));
