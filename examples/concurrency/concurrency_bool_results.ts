function taskBoolToNumber(value: boolean): number {
  if (value) {
    return 42;
  }
  return 0;
}

const taskBoolResult = spawn((): boolean => true);
const taskBoolValue: boolean = join(taskBoolResult);
console.log(taskBoolToNumber(taskBoolValue));
