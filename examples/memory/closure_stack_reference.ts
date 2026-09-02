function localMessage(base: string): string {
  const render = (): string => base + "-ok";
  let fn: () => string = render;

  let churn = "";
  for (let i: number = 0; i < 512; i++) {
    churn = churn + "0123456789abcdef";
  }

  return fn();
}

console.log(localMessage("root"));
