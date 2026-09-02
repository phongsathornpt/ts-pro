let total: number = 0;
for (let i: number = 0; i < 5; i++) {
  total = total + i;
}

let j: number = 0;
while (j < 3) {
  total = total + 10;
  j++;
}

if (total === 40) {
  console.log(total);
} else {
  console.log(0);
}
