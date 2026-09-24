import type { Field } from "../lib/types";

interface Props {
  field: Field;
  value: unknown;
  onChange: (v: unknown) => void;
}

const isUnset = (v: unknown) => v === undefined || v === null || v === "";

function defaultText(f: Field): string {
  if (f.default === undefined) return f.required ? "required" : "optional";
  return `default ${Array.isArray(f.default) ? f.default.join(", ") : String(f.default)}`;
}

function NumberInput({ field, value, onChange }: Props) {
  return (
    <input
      type="number"
      step="any"
      value={isUnset(value) ? "" : String(value)}
      placeholder={defaultText(field)}
      onChange={(e) => onChange(e.target.value === "" ? undefined : Number(e.target.value))}
    />
  );
}

function ChoiceInput({ field, value, onChange }: Props) {
  const current = Array.isArray(value) ? value[0] : value;
  const pick = (v: string) => {
    if (v === "") return onChange(undefined);
    onChange(field.multi ? [v] : v);
  };
  return (
    <select value={isUnset(current) ? "" : String(current)} onChange={(e) => pick(e.target.value)}>
      <option value="">{defaultText(field)}</option>
      {(field.options ?? []).filter((o) => o !== "").map((o) => (
        <option key={o} value={o}>
          {o}
        </option>
      ))}
    </select>
  );
}

function FlagInput({ field, value, onChange }: Props) {
  const pick = (v: string) => onChange(v === "" ? undefined : v === "true");
  return (
    <select value={isUnset(value) ? "" : String(value)} onChange={(e) => pick(e.target.value)}>
      <option value="">{defaultText(field)}</option>
      <option value="true">true</option>
      <option value="false">false</option>
    </select>
  );
}

function asText(value: unknown): string {
  if (Array.isArray(value)) return value.join(", ");
  return isUnset(value) ? "" : String(value);
}

function TextInput({ field, value, onChange }: Props) {
  const list = field.type === "list";
  const text = asText(value);
  const commit = (v: string) => {
    if (v.trim() === "") return onChange(undefined);
    onChange(list ? v.split(",").map((s) => s.trim()).filter(Boolean) : v);
  };
  return <input type="text" value={text} placeholder={defaultText(field)} onChange={(e) => commit(e.target.value)} />;
}

function Control(props: Props) {
  switch (props.field.type) {
    case "number":
      return <NumberInput {...props} />;
    case "choice":
      return <ChoiceInput {...props} />;
    case "boolean":
      return <FlagInput {...props} />;
    default:
      return <TextInput {...props} />;
  }
}

export function FieldInput(props: Props) {
  const { field, value } = props;
  const missing = field.required && isUnset(value);
  return (
    <label className={`field${missing ? " missing" : ""}`} title={field.path ? `Terraform: ${field.path}` : undefined}>
      <span className="field-label">
        {field.label}
        {field.unit && <span className="field-unit">{field.unit}</span>}
      </span>
      <Control {...props} />
      {field.hint && <span className="field-hint">{field.hint}</span>}
    </label>
  );
}
