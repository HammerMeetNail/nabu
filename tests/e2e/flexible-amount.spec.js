import {test,expect} from '@playwright/test';
import {fixture} from './review-fixtures.js';

test('medication amount is a free number separate from common unit choices',async({page})=>{
  const {chore}=await fixture(page,{name:'Baby Meds',metricUnit:'mg'});
  await page.locator(`[data-home-chore-id="${chore.id}"]`).click();
  const amount=page.locator('#log-volume'),unit=page.locator('#log-metric-unit');
  await expect(amount).toHaveAttribute('type','number');
  for(const value of ['mcg','mg','g','mL','L','drops','tablets','capsules','puffs','units']) {
    await expect(unit.locator(`option[value="${value}"]`)).toHaveCount(1);
  }
  await amount.fill('200');
  await unit.selectOption('mL');
  await expect(amount).toHaveValue('200');
  await unit.selectOption('L');
  await expect(amount).toHaveValue('200');
  await amount.fill('12345');
  await page.locator('[data-action="save-log"]').click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await page.reload();await page.locator('[data-nav="activity"]').click();
  const row=page.locator('[data-action="view-log"]').filter({hasText:'12345 L'});
  await row.click();
  await expect(amount).toHaveValue('12345');await expect(unit).toHaveValue('L');
  await amount.fill('200');await unit.selectOption('mL');
  await page.getByRole('button',{name:'Cancel',exact:true}).click();
  await row.click();await expect(amount).toHaveValue('12345');await expect(unit).toHaveValue('L');
  await amount.fill('200');await unit.selectOption('mL');
  await page.locator('[data-action="save-log"]').click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await page.reload();await page.locator('[data-nav="activity"]').click();
  await expect(page.locator('[data-action="view-log"]')).toContainText('200 mL');
});
