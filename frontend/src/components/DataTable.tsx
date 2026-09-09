import { For } from 'solid-js'

export interface Column<T> {
  header: string
  cell: (row: T) => any
}

export function DataTable<T>(props: { columns: Column<T>[]; rows: T[]; rowKey: (row: T) => string }) {
  return (
    <table>
      <thead>
        <tr>
          <For each={props.columns}>{(col) => <th>{col.header}</th>}</For>
        </tr>
      </thead>
      <tbody>
        <For each={props.rows}>
          {(row) => (
            <tr data-testid="data-table-row" data-row-key={props.rowKey(row)}>
              <For each={props.columns}>{(col) => <td>{col.cell(row)}</td>}</For>
            </tr>
          )}
        </For>
      </tbody>
    </table>
  )
}
