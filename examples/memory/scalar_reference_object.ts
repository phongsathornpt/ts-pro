interface ScalarReferenceHolder {
  value: string;
}

function scalarReferenceValue(flag: boolean): string {
  const holder: ScalarReferenceHolder = { value: "start" };
  if (flag) {
    holder.value = "alpha";
  } else {
    holder.value = "beta";
  }

  const selected = holder.value;
  let churn = "";
  for (let i: number = 0; i < 512; i++) {
    churn = churn + "0123456789abcdef";
  }
  return selected;
}

console.log(scalarReferenceValue(false));
