const taskFunctionResult = spawn((): (() => number) => {
  const innerTaskFunction = (): number => 42;
  return innerTaskFunction;
});
const taskFunctionValue: () => number = join(taskFunctionResult);
console.log(taskFunctionValue());
