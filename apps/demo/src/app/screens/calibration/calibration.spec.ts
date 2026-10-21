import { TestBed } from '@angular/core/testing';
import { beforeEach, describe, expect, it } from 'vitest';
import { FakeVitals, WARMING_UP, estimate } from '../../testing/fake-vitals';
import { VITALS_FACTORY, VitalsService } from '../../vitals.service';
import { Calibration } from './calibration';

describe('Calibration', () => {
  let fake: FakeVitals;

  beforeEach(async () => {
    fake = new FakeVitals();
    TestBed.configureTestingModule({ providers: [{ provide: VITALS_FACTORY, useValue: () => fake }] });
    await TestBed.inject(VitalsService).consentAndStart(document.createElement('video'));
  });

  it('shows the live reason text and updates it', async () => {
    const fixture = TestBed.createComponent(Calibration);
    const el = fixture.nativeElement as HTMLElement;
    await fixture.whenStable();
    expect(el.querySelector('[aria-live]')?.textContent.trim()).toBe('');

    fake.emit('estimate', WARMING_UP);
    await fixture.whenStable();
    expect(el.querySelector('[aria-live]')?.textContent.trim()).toBe('Getting a steady signal…');

    fake.emit('estimate', estimate({ confidence: 'invalid', reason: 'low_light' }));
    await fixture.whenStable();
    expect(el.querySelector('[aria-live]')?.textContent.trim()).toBe('More light, please');
  });

  it('shows the warming-up copy for an invalid estimate without a reason', async () => {
    const fixture = TestBed.createComponent(Calibration);
    const el = fixture.nativeElement as HTMLElement;
    fake.emit('estimate', estimate({ confidence: 'invalid', reason: null }));
    await fixture.whenStable();
    expect(el.querySelector('[aria-live]')?.textContent.trim()).toBe('Getting a steady signal…');
  });
});
