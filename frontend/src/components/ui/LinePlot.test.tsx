import { fireEvent, render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { LinePlot } from "./LinePlot";

it("shows a single negative observation and its original decimal label", () => {
  const select=vi.fn();
  const {container}=render(<LinePlot label="Saldo" dates={["2026-10-22"]} height={240} formatDate={(day)=>day} series={[{key:"cash",label:"Saldo",color:"#123",values:["-100.000000001"]}]} tooltip={()=><p>Saldo: -100.000000001</p>} onSelect={select} />);
  expect(container.querySelector("circle")).toHaveAttribute("r","3");
  expect(container.querySelector("svg")?.innerHTML).not.toMatch(/NaN|Infinity/);
  fireEvent.change(screen.getByRole("combobox"),{target:{value:"0"}});
  expect(screen.getByText("Saldo: -100.000000001")).toBeVisible();
  fireEvent.click(screen.getByRole("button",{name:"Ver transações do dia"}));
  expect(select).toHaveBeenCalledWith(0);
});
it("keeps unavailable observations as gaps and renders an empty series safely", () => {
  const props={label:"Saldo",height:240,formatDate:(day:string)=>day,tooltip:()=>null};
  const {container,rerender}=render(<LinePlot {...props} dates={["2026-10-21","2026-10-22","2026-10-23"]} series={[{key:"cash",label:"Saldo",color:"#123",values:["0",null,"0"]}]} />);
  expect(container.querySelector("path")?.getAttribute("d")?.match(/M/g)).toHaveLength(2);
  rerender(<LinePlot {...props} dates={[]} series={[]} />);
  expect(container.querySelector("svg")?.innerHTML).not.toMatch(/NaN|Infinity/);
});
