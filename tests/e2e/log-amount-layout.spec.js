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
    await expect(page.getByRole('combobox', {name: 'Choose amount', exact: true})).toBeFocused();
    await page.keyboard.press('Tab');
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

test('default Feed Baby keeps its shared unit beside the amount inputs', async ({page}) => {
  const {headers} = await fixture(page);
  expect((await page.request.post('/api/chores/seed-defaults', {headers})).ok()).toBeTruthy();
  const {chores} = await (await page.request.get('/api/chores')).json();
  const chore = chores.find(chore => chore.name === 'Feed Baby');
  await page.reload();
  await page.locator(`[data-home-chore-id="${chore.id}"]`).click();

  const formula = page.getByRole('spinbutton', {name: '🍼 formula amount'});
  const breast = page.getByRole('spinbutton', {name: '🤱 breast amount'});
  const unit = page.getByLabel('Unit', {exact: true});
  const expectAligned = async () => {
    const amountBox = await formula.boundingBox();
    const unitBox = await unit.boundingBox();
    expect(Math.abs(amountBox.y - unitBox.y)).toBeLessThanOrEqual(1);
    expect(Math.abs(amountBox.height - unitBox.height)).toBeLessThanOrEqual(1);
    expect(amountBox.height).toBeGreaterThanOrEqual(44);
    expect(amountBox.width).toBeGreaterThanOrEqual(76);
    expect(unitBox.width).toBeGreaterThanOrEqual(96);
    expect(unitBox.x).toBeGreaterThanOrEqual(amountBox.x + amountBox.width);
    expect(unitBox.x + unitBox.width).toBeLessThanOrEqual(page.viewportSize().width);
  };

  for (const width of [320, 390, 960]) {
    await page.setViewportSize({width, height: 844});
    await expectAligned();
  }
  await page.setViewportSize({width: 640, height: 1000});
  const zoom = await page.addStyleTag({content: 'html { zoom: 2; }'});
  await expectAligned();
  await zoom.evaluate(element => element.remove());
  await page.setViewportSize({width: 390, height: 844});

  await formula.fill('120');
  await page.getByRole('button', {name: '🤱 breast', exact: true}).click();
  await breast.fill('30');
  await expect(unit).toHaveCount(1);
  await expectAligned();
  await page.locator('[data-action="save-log"]').click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await page.reload();
  const {logs} = await (await page.request.get('/api/logs/history')).json();
  const log = logs.find(log => log.choreId === chore.id);
  expect(log).toMatchObject({metricUnit: 'mL', indicatorVolumes: {'🍼 formula': 120, '🤱 breast': 30}});
  await page.locator('[data-nav="activity"]').click();
  await page.locator(`[data-action="view-log"][data-log-id="${log.id}"]`).click();
  await expect(formula).toHaveValue('120');
  await expect(breast).toHaveValue('30');
  await expectAligned();
  await unit.selectOption('oz');
  await expect(formula).toHaveValue('120');
  await expect(breast).toHaveValue('30');
  await page.getByRole('button', {name: 'Cancel', exact: true}).click();
  await page.locator(`[data-action="view-log"][data-log-id="${log.id}"]`).click();
  await expect(unit).toHaveValue('mL');
  await expectAligned();
});

test('medication with custom types keeps one inline unit and preserves toggled amounts', async ({page}) => {
  const {chore} = await fixture(page, {
    name: 'Cat Meds', metricUnit: 'mg',
    indicatorLabels: ['Morning dose', 'Evening dose'],
    indicatorDefaults: ['Morning dose', 'Evening dose'],
  });
  await page.locator(`[data-home-chore-id="${chore.id}"]`).click();
  const morning = page.getByRole('spinbutton', {name: 'Morning dose amount'});
  const evening = page.getByRole('spinbutton', {name: 'Evening dose amount'});
  const unit = page.getByLabel('Unit', {exact: true});
  for (const width of [320, 390, 960]) {
    await page.setViewportSize({width, height: 844});
    const amountBox = await morning.boundingBox(), unitBox = await unit.boundingBox();
    expect(Math.abs(amountBox.y - unitBox.y)).toBeLessThanOrEqual(1);
    expect(Math.abs(amountBox.height - unitBox.height)).toBeLessThanOrEqual(1);
    expect(amountBox.height).toBeGreaterThanOrEqual(44);
    expect(unitBox.x).toBeGreaterThanOrEqual(amountBox.x + amountBox.width);
    expect(unitBox.x + unitBox.width).toBeLessThanOrEqual(width);
  }
  await unit.selectOption('tablets');
  await morning.fill('2');
  await evening.fill('1');
  await page.getByRole('button', {name: 'Morning dose', exact: true}).click();
  await expect(morning).toBeHidden();
  await expect(unit).toBeVisible();
  await expect(unit).toHaveCount(1);
  await evening.press('Tab');
  await expect(page.getByRole('combobox', {name: 'Choose Evening dose amount', exact: true})).toBeFocused();
  await page.keyboard.press('Tab');
  await expect(unit).toBeFocused();
  await page.getByRole('button', {name: 'Morning dose', exact: true}).click();
  await expect(morning).toHaveValue('2');
  await page.locator('[data-action="save-log"]').click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  const {logs} = await (await page.request.get('/api/logs/history')).json();
  expect(logs.find(log => log.choreId === chore.id)).toMatchObject({
    metricUnit: 'tablets', indicatorVolumes: {'Morning dose': 2, 'Evening dose': 1},
  });
});
