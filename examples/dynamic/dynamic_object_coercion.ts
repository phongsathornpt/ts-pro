function dynamicAdd(left: any, right: any): any {
  return left + right;
}

function dynamicSub(left: any, right: any): number {
  return left - right;
}

function dynamicLooseEqual(left: any, right: any): any {
  return left == right;
}

console.log(dynamicAdd({ value: 1 }, 2));
console.log(dynamicAdd("array=", [1, 2]));
console.log(dynamicSub([5], 2));
console.log(dynamicLooseEqual({ value: 1 }, "[object Object]"));
