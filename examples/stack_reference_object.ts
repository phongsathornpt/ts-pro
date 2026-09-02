interface StackReferenceHolder {
  value: string;
}

function stackReferenceValue(): string {
  const holder: StackReferenceHolder = { value: "initial" };
  holder.value = "kept";

  let churn = "";
  for (let i: number = 0; i < 2048; i++) {
    churn = churn + "0123456789abcdef";
  }

  return holder.value;
}

console.log(stackReferenceValue());
