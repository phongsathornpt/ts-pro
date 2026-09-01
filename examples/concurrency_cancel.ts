function cancellationToNumber(value: boolean): number {
  if (value) return 1;
  return 0;
}

const cancellationGate = channel<boolean>(0);
const cancelledWorker = spawn((): void => {
  channelRecv(cancellationGate);
  console.log(cancellationToNumber(taskCancelled()));
});

cancelTask(cancelledWorker);
const cancellationRelease = spawn((): void => {
  channelSend(cancellationGate, true);
});

join(cancelledWorker);
join(cancellationRelease);
