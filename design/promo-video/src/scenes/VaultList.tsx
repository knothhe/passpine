import React from "react";
import { ink, muted } from "../theme";

type Entry = { initials: string; name: string; path: string; fresh: boolean };

/** Entry list column shared by the sync and import scenes. */
export const VaultList = ({
  title,
  entries,
  freshHeight = 1,
  selected,
}: {
  title: string;
  entries: readonly Entry[];
  freshHeight?: number;
  selected?: string;
}) => (
  <div style={{ padding: "22px 18px" }}>
    <div style={{ fontSize: 24, fontWeight: 700, padding: "0 8px 18px" }}>{title}</div>
    {entries.map((e) => {
      const h = e.fresh ? freshHeight : 1;
      return (
        <div key={e.name} style={{ height: 76 * h, opacity: h, overflow: "hidden", transform: `translateX(${(1 - h) * -30}px)` }}>
          <div
            style={{
              height: 68,
              display: "flex",
              alignItems: "center",
              gap: 16,
              padding: "0 12px",
              borderRadius: 12,
              background: selected === e.name ? "#ebebe9" : "transparent",
            }}
          >
            <div style={{ width: 46, height: 46, borderRadius: 10, background: "#f0f0ee", display: "grid", placeItems: "center", fontSize: 15, fontWeight: 700, color: "#555" }}>
              {e.initials}
            </div>
            <div>
              <div style={{ fontSize: 20, fontWeight: 600, color: ink }}>{e.name}</div>
              <div style={{ fontSize: 15, color: muted, marginTop: 2 }}>{e.path}</div>
            </div>
            {e.fresh && <div style={{ marginLeft: "auto", width: 10, height: 10, borderRadius: 5, background: "#2f7bf6", opacity: selected === e.name ? 0 : 1 }} />}
          </div>
        </div>
      );
    })}
  </div>
);
