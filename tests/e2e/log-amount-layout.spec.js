import {test, expect} from '@playwright/test';
import {fixture, postLog} from './review-fixtures.js';

for (const [name, metricUnit] of [['Baby Meds', 'mg'], ['Bottle', 'mL']]) {
  test(`${name} keeps amount and unit aligned across screen sizes and editing`, async ({page}) => {
    const {chore} = await fixture(page, {name, metricUnit});
    const log = await postLog(page, chore, {volumeML: 25});
    await page.reload();
    await page.locator(`[data-home-chore-id="${chore.id}"]`).click();

    const amount = page.getByLabel('Amount', {exact: true});
    const unit = page.getByLabel('Unit', {exact: true});
    await expect(page.locator('.volume-recent-chip')).toHaveText(`25 ${metricUnit}`);

    const expectAligned = async () => {
      const amountBox = await amount.boundingBox();
      const unitBox = await unit.boundingBox();
      expect(amountBox).not.toBeNull();
      expect(unitBox).not.toBeNull();
      expect(Math.abs(amountBox.y - unitBox.y)).toBeLessThanOrEqual(1);
      expect(Math.abs(amountBox.height - unitBox.height)).toBeLessThanOrEqual(1);
      expect(amountBox.height).toBeGreaterThanOrEqual(44);
      expect(unitBox.x).toBeGreaterThanOrEqual(amountBox.x + amountBox.width);
      expect(amountBox.width).toBeGreaterThanOrEqual(100);
      expect(unitBox.width).toBeGreaterThanOrEqual(100);
      expect(unitBox.x + unitBox.width).toBeLessThanOrEqual(page.viewportSize().width);
    };

    for (const width of [320, 390, 960]) {
      await page.setViewportSize({width, height: 844});
      await expectAligned();
    }
    await page.setViewportSize({width: 640, height: 1000});
    const zoom = await page.addStyleTag({content: 'html { zoom: 2; }'});
    await expectAligned();
    await zoom.evaluate(el => el.remove());

    await amount.fill('37');
    await amount.press('Tab');
    await expect(unit).toBeFocused();
    await unit.selectOption('capsules');
    await expect(amount).toHaveValue('37');
    await page.getByRole('button', {name: 'Cancel', exact: true}).click();

    await page.locator('[data-nav="activity"]').click();
    await page.locator(`[data-action="view-log"][data-log-id="${log.id}"]`).click();
    await expect(amount).toHaveValue('25');
    await expect(unit).toHaveValue(metricUnit);
    await expectAligned();
  });
}
