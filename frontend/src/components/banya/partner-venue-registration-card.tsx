"use client";

import type { ReactNode } from "react";
import { Building2, FileText, Link2 } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { CityCombobox } from "@/components/banya/city-combobox";
import { AddressSuggest } from "@/components/banya/address-suggest";
import { PhoneInput } from "@/components/banya/phone-input";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { venueTypeBadgeLabel, type PartnerVenueFormValues } from "@/lib/partner-venue";

type Props = {
  values: PartnerVenueFormValues;
  onChange: (patch: Partial<PartnerVenueFormValues>) => void;
  /** В кабинете владельца нужен телефон заведения; при регистрации партнёра телефон берётся со следующего шага */
  showVenuePhone?: boolean;
  venuePhone?: string;
  onVenuePhoneChange?: (v: string) => void;
  /** Редактирование карточки: тип только для отображения (PATCH тип не принимает). */
  venueTypeReadOnly?: boolean;
  /** Ограничение длины описания и счётчик символов (кабинет владельца). */
  descriptionMaxLength?: number;
  /** Например часы работы — между телефоном и блоком проверки владельца. */
  slotAfterPhone?: ReactNode;
  wrapperClassName?: string;
  footer?: ReactNode;
  caption?: string | null;
};

export function PartnerVenueRegistrationCard({
  values: v,
  onChange,
  showVenuePhone,
  venuePhone,
  onVenuePhoneChange,
  venueTypeReadOnly,
  descriptionMaxLength,
  slotAfterPhone,
  wrapperClassName,
  footer,
  caption,
}: Props) {
  const descLimit = descriptionMaxLength ?? undefined;
  return (
    <div className={cn("mx-auto max-w-lg", wrapperClassName)}>
      <Card className="border-border">
        <CardHeader className="text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
            <Building2 className="h-6 w-6 text-primary" />
          </div>
          <CardTitle className="text-2xl text-card-foreground">Расскажите о заведении</CardTitle>
          <CardDescription>Эти данные помогут нам создать страницу вашей бани</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="venueName">Название заведения</Label>
            <Input
              id="venueName"
              placeholder="Например: Русская Банька на Дровах"
              value={v.venueName}
              onChange={(e) => onChange({ venueName: e.target.value })}
              required
            />
          </div>

          <div className="space-y-2">
            <Label>Тип заведения</Label>
            {venueTypeReadOnly ? (
              <>
                <div className="flex h-10 items-center">
                  <Badge variant="secondary">{venueTypeBadgeLabel(v.venueType)}</Badge>
                </div>
                <p className="text-xs text-muted-foreground">Тип после создания не меняется.</p>
              </>
            ) : (
              <Select value={v.venueType} onValueChange={(type) => onChange({ venueType: type })}>
                <SelectTrigger>
                  <SelectValue placeholder="Выберите тип" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="banya">Баня</SelectItem>
                  <SelectItem value="sauna">Сауна</SelectItem>
                  <SelectItem value="hammam">Хаммам</SelectItem>
                </SelectContent>
              </Select>
            )}
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-2">
              <Label>Город</Label>
              <CityCombobox value={v.venueCity} onChange={(city) => onChange({ venueCity: city })} />
            </div>
            <div className="space-y-2">
              <Label>Адрес</Label>
              <AddressSuggest
                value={v.venueAddress}
                onChange={(address) => onChange({ venueAddress: address })}
                city={v.venueCity}
                placeholder="ул. Банная, д. 15"
              />
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="venueDescription">Описание</Label>
            <Textarea
              id="venueDescription"
              placeholder="Расскажите, чем ваше заведение особенное..."
              value={v.venueDescription}
              onChange={(e) => {
                const raw = e.target.value;
                const next =
                  descLimit !== undefined ? raw.slice(0, descLimit) : raw;
                onChange({ venueDescription: next });
              }}
              rows={descLimit !== undefined ? 6 : 3}
              maxLength={descLimit}
              className={descLimit !== undefined ? "min-h-[150px] resize-y" : undefined}
            />
            {descLimit !== undefined ? (
              <p className="text-sm text-muted-foreground">
                {v.venueDescription.length} / {descLimit} символов
              </p>
            ) : null}
          </div>

          {showVenuePhone ? (
            <div className="space-y-2">
              <Label htmlFor="venueGuestPhone">Телефон заведения для гостей</Label>
              <PhoneInput
                id="venueGuestPhone"
                value={venuePhone ?? ""}
                onChange={(phone) => onVenuePhoneChange?.(phone)}
                required
              />
              <p className="text-xs text-muted-foreground">
                Отображается на карточке заведения и после модерации в каталоге.
              </p>
            </div>
          ) : null}

          {slotAfterPhone}

          <div className="rounded-lg border border-border bg-muted/30 p-4 space-y-3">
            <div className="flex items-center gap-2 text-sm font-medium text-foreground">
              <FileText className="h-4 w-4 text-primary" />
              Данные для проверки владельца
            </div>
            <p className="text-xs text-muted-foreground leading-relaxed">
              Чтобы отсечь фиктивные заявки, мы сверяем ИНН и ОГРН/ОГРНИП с открытыми реестрами (ЕГРЮЛ/ЕГРИП) и смотрим публичную карточку заведения на картах. Без этого модерация невозможна.
            </p>
            <div className="space-y-2">
              <Label htmlFor="legalEntityName">Полное наименование ИП или организации</Label>
              <Input
                id="legalEntityName"
                placeholder="Как в выписке ЕГРЮЛ / ЕГРИП"
                value={v.legalEntityName}
                onChange={(e) => onChange({ legalEntityName: e.target.value })}
                required
              />
            </div>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="inn">ИНН</Label>
                <Input
                  id="inn"
                  inputMode="numeric"
                  placeholder="10 цифр (ООО) или 12 (ИП)"
                  value={v.inn}
                  onChange={(e) => onChange({ inn: e.target.value })}
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="ogrn">ОГРН или ОГРНИП</Label>
                <Input
                  id="ogrn"
                  inputMode="numeric"
                  placeholder="13 цифр (ОГРН) или 15 (ОГРНИП)"
                  value={v.ogrn}
                  onChange={(e) => onChange({ ogrn: e.target.value })}
                  required
                />
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="publicListingUrl" className="flex items-center gap-1.5">
                <Link2 className="h-3.5 w-3.5" />
                Ссылка на карточку на картах
              </Label>
              <Input
                id="publicListingUrl"
                type="url"
                placeholder="https://yandex.ru/maps/org/... или 2gis.ru/..."
                value={v.publicListingUrl}
                onChange={(e) => onChange({ publicListingUrl: e.target.value })}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="verificationNote">Комментарий для модератора (необязательно)</Label>
              <Textarea
                id="verificationNote"
                placeholder="Например: торговая марка отличается от юр. названия, доступ к телефону организации..."
                value={v.verificationNote}
                onChange={(e) => onChange({ verificationNote: e.target.value })}
                rows={2}
              />
            </div>
          </div>

          {footer ?? null}
        </CardContent>
      </Card>

      {caption ? (
        <p className="mt-4 text-center text-sm text-muted-foreground">{caption}</p>
      ) : null}
    </div>
  );
}
