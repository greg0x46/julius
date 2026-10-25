let openDialogs = 0;
let previousOverflow = "";
export function lockDialogScroll() {
  if (openDialogs++ === 0) {
    previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
  }
  return () => {
    if (--openDialogs === 0) document.body.style.overflow = previousOverflow;
  };
}
