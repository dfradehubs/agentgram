"use client";

import type { AgentApiKeyRule } from "@/lib/types";
import { Button } from "@/components/ui/button";
import { Plus, Trash2 } from "lucide-react";
import { useT } from "@/lib/i18n";

interface ApiKeyRulesEditorProps {
  rules: AgentApiKeyRule[];
  onChange: (rules: AgentApiKeyRule[]) => void;
}

export function ApiKeyRulesEditor({ rules, onChange }: ApiKeyRulesEditorProps) {
  const t = useT();

  const updateRule = (index: number, patch: Partial<AgentApiKeyRule>) =>
    onChange(rules.map((rule, i) => (i === index ? { ...rule, ...patch } : rule)));

  const removeRule = (index: number) =>
    onChange(rules.filter((_, i) => i !== index));

  const addRule = () =>
    onChange([...rules, { subject_type: "group", subject: "", api_key: "" }]);

  return (
    <div>
      <label className="mb-1 block text-sm font-medium">{t("admin.agents.apiKeyRules")}</label>
      <p className="mb-2 text-xs text-muted-foreground">{t("admin.agents.apiKeyRulesHelp")}</p>

      <div className="space-y-2">
        {rules.map((rule, i) => (
          <div key={i} className="flex items-center gap-2">
            <select
              className="rounded-md border bg-background px-2 py-2 text-sm"
              value={rule.subject_type}
              onChange={e => updateRule(i, { subject_type: e.target.value as AgentApiKeyRule["subject_type"] })}
            >
              <option value="user">{t("admin.agents.apiKeyRuleUser")}</option>
              <option value="group">{t("admin.agents.apiKeyRuleGroup")}</option>
            </select>
            <input
              className="min-w-0 flex-1 rounded-md border bg-background px-3 py-2 text-sm"
              value={rule.subject}
              onChange={e => updateRule(i, { subject: e.target.value })}
              placeholder={rule.subject_type === "user"
                ? t("admin.agents.apiKeyRuleSubjectUser")
                : t("admin.agents.apiKeyRuleSubjectGroup")}
              required
            />
            <input
              className="min-w-0 flex-1 rounded-md border bg-background px-3 py-2 font-mono text-sm"
              type="password"
              value={rule.api_key}
              onChange={e => updateRule(i, { api_key: e.target.value })}
              placeholder={t("admin.agents.apiKeyRuleKey")}
              autoComplete="off"
              required
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              onClick={() => removeRule(i)}
              title={t("admin.agents.apiKeyRuleRemove")}
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        ))}
      </div>

      <Button type="button" variant="outline" size="sm" className="mt-2" onClick={addRule}>
        <Plus className="mr-1 h-3.5 w-3.5" />
        {t("admin.agents.apiKeyRulesAdd")}
      </Button>
    </div>
  );
}
