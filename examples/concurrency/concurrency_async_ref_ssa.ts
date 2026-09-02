async function asyncStringSSA(base: string): Promise<string> {
  const prefix = "pre:";
  const joined = prefix + base;
  sleep(1);
  return joined + ":done";
}

async function asyncAnySSA(base: any): Promise<any> {
  const suffix: any = "-any";
  const joined: any = base + suffix;
  sleep(1);
  return joined;
}

console.log(join(asyncStringSSA("value")));
console.log(join(asyncAnySSA("value")));
