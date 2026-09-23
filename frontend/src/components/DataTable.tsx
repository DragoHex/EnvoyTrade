import { For, Index, Show } from 'solid-js'

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
  emptyMessage?: string
  renderExpandedRow?: (row: T) => any
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
        <Show
          when={props.rows.length > 0}
          fallback={
            <Show when={props.emptyMessage}>
              <tr>
                <td
                  colspan={props.columns.length}
                  class="table-empty-cell"
                  style={{ color: 'var(--color-text-secondary)', 'text-align': 'center', padding: '1.5rem' }}
                >
                  {props.emptyMessage}
                </td>
              </tr>
            </Show>
          }
        >
          <Index each={props.rows}>
            {(row) => (
              <>
                <tr data-testid="data-table-row" data-row-key={props.rowKey(row())}>
                  <For each={props.columns}>
                    {(col) => <td class={col.cellClass}>{col.cell(row())}</td>}
                  </For>
                </tr>
                {props.renderExpandedRow ? props.renderExpandedRow(row()) : null}
              </>
            )}
          </Index>
        </Show>
      </tbody>
    </table>
  )
}
