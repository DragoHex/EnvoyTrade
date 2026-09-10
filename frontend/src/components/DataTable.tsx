import { For, Index } from 'solid-js'

export interface Column<T> {
  header: string
  headerClass?: string
  cellClass?: string
  cell: (row: T) => any
}

export function DataTable<T>(props: {
  columns: Column<T>[]
  rows: T[]
  rowKey: (row: T) => string
  tableClass?: string
}) {
  return (
    <table class={props.tableClass}>
      <thead>
        <tr>
          <For each={props.columns}>
            {(col) => <th class={col.headerClass}>{col.header}</th>}
          </For>
        </tr>
      </thead>
      <tbody>
        <Index each={props.rows}>
          {(row) => (
            <tr data-testid="data-table-row" data-row-key={props.rowKey(row())}>
              <For each={props.columns}>
                {(col) => <td class={col.cellClass}>{col.cell(row())}</td>}
              </For>
            </tr>
          )}
        </Index>
      </tbody>
    </table>
  )
}
